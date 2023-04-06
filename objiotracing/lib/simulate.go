package lib

import (
	"fmt"

	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
	"github.com/dgryski/go-clockpro"
	"github.com/dgryski/go-s4lru"
	"github.com/dgryski/go-tinylfu"
)

type ReplacementPolicy int

const (
	ClockPro ReplacementPolicy = iota
	S4LRU
	TinyLFU
	// TODO(josh): Implement. Can use https://github.com/dgryski/go-tinylfu/blob/master/lru.go.
	LRU
)

func (p ReplacementPolicy) String() string {
	if p == ClockPro {
		return "ClockPro"
	} else if p == S4LRU {
		return "S4LRU"
	} else if p == TinyLFU {
		return  "TinyLFU"
	} else if p == LRU {
		return "LRU"
	} else {
		panic("not implemented")
	}
}

// Note that this is used a key into a cache. Avoid pointer fields.
// See resultCacheKey for more.
type Config struct {
	Policy                   ReplacementPolicy
	// If 0, then (i) assume reads & writes will always be in units of pebble sstable
	// block size and (ii) do caching in units of pebble sstable blocks.
	// TODO(josh): I am unsure if the first assumption is okay to make. A related question
	// I have is whether the existing in-memory pebble block cache caches in units of pebble
	// sstable blocks.
	BlockSize                int64
	CacheSize                int
	// Must be set >0 if Policy == TinyLFU. Else must be 0.
	TinyLFUSamples           int
	WriteThru                bool
	CacheUserFacingReadsOnly bool
	L5AndL6Only              bool
}

func (c *Config) String() string {
	return fmt.Sprintf("%+v", c)
}

type Results struct {
	Hits   int
	Misses int
}

func Simulate(traceID string, it iterator, config Config) (*Results, error) {
	if config.Policy == S4LRU {
		// Requires that size is divisible by four. Must adjust before
		// interacting with the cache.
		config.CacheSize = config.CacheSize / 4 * 4
	}

	results, ok := resultCache[resultCacheKey{
		traceID: traceID,
		config:  config,
	}]
	if ok {
		return &results, nil
	}

	results = Results{}
	var cache cache
	if config.Policy == ClockPro {
		cache = clockpro.New(config.CacheSize)
	} else if config.Policy == S4LRU {
		cache = &wrappedS4LRU{s4lru.New(config.CacheSize)}
	} else if config.Policy == TinyLFU {
		if config.TinyLFUSamples == 0 {
			panic("samples expected to be set but not set")
		}
		cache = &wrappedTinyLFU{tinylfu.New(config.CacheSize, config.TinyLFUSamples)}
	} else {
		panic("replacement policy not implemented")
	}
	if config.Policy != TinyLFU && config.TinyLFUSamples != 0 {
		panic("sampled expected to not be set (set to 0) but is set")
	}

	for {
		trace, err := it.NextBatch()
		if err != nil {
			return nil, err
		}
		if trace == nil {
			break
		}

		for _, e := range trace {
			if config.L5AndL6Only {
				if e.LevelPlusOne <= 5 {
					continue
				}
			}
			// TODO(josh): We may want to ignore RecordCacheHitOps, or at least
			// not call Set when they come up. Some discussion about this is at
			// https://github.com/RaduBerinde/pebble_analysis/pull/1#discussion_r1158823825
			if e.Op == objiotracing.ReadOp || e.Op == objiotracing.RecordCacheHitOp {
				if config.CacheUserFacingReadsOnly {
					if e.Reason != objiotracing.UnknownReason {
						continue
					}
				}
				offset := e.Offset
				if config.BlockSize != 0 {
					// TODO(josh): The end of a read may hit a different "cache block" than
					// the start of a read. This code currently only simulates reading the
					// first "cache block".
					offset = offset / config.BlockSize
				}
				k := fmt.Sprintf("%v/%v", e.FileNum, offset)
				v := cache.Get(k)
				if v == nil {
					results.Misses++
					cache.Set(k, true)
				} else  {
					results.Hits++
				}
			}
			if config.WriteThru {
				if e.Op == objiotracing.WriteOp {
					// TODO(josh): The end of a write may hit a different "cache block" than
					// the start of a write. This code currently only simulates writing the
					// first "cache block" out.
					offset := e.Offset
					if config.BlockSize != 0 {
						offset = offset / config.BlockSize
					}
					k := fmt.Sprintf("%v/%v", e.FileNum, e.Offset)
					cache.Set(k, true)
				}
			}
		}
	}

	resultCache[resultCacheKey{
		traceID: traceID,
		config:  config,
	}] = results
	return &results, nil
}

type cache interface {
	Get(key string) interface{}
	Set(key string, value interface{})
}

type wrappedS4LRU struct {
	c *s4lru.Cache
}

func (c *wrappedS4LRU) Get(key string) interface{} {
	val, ok := c.c.Get(key)
	if !ok {
		return nil
	}
	return val
}

func (c *wrappedS4LRU) Set(key string, value interface{}) {
	c.c.Set(key, value)
}

type wrappedTinyLFU struct {
	c *tinylfu.T
}

func (c *wrappedTinyLFU) Get(key string) interface{} {
	val, ok := c.c.Get(key)
	if !ok {
		return nil
	}
	return val
}

func (c *wrappedTinyLFU) Set(key string, value interface{}) {
	c.c.Add(key, value)
}

type iterator interface {
	NextBatch() ([]objiotracing.Event, error)
}

// TODO(josh): Make caching persistent instead of in-memory.
// TODO(josh): Enable adding new options to Config without
// needing to clear out caches.
type resultCacheKey struct {
	traceID string
	config  Config
}

var resultCache = map[resultCacheKey]Results{}

