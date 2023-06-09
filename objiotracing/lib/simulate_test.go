package lib

import (
	"fmt"
	"testing"

	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
	"github.com/stretchr/testify/require"
)

type wrappedTrace struct {
	inner []objiotracing.Event
	done  bool
}

func (t *wrappedTrace) NextBatch() ([]objiotracing.Event, error) {
	if t.done {
		return nil, nil
	}
	t.done = true
	return t.inner, nil
}

func TestSimulate(t *testing.T) {
	for i, policy := range []ReplacementPolicy{ClockPro, S4LRU, TinyLFU} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var config Config
			var trace []objiotracing.Event
			t.Run("basic", func(t *testing.T) {
				config = Config{
					Policy: policy,
					CacheSize: 1024,
				}
				if policy == TinyLFU {
					samples := 10 * config.CacheSize
					config.TinyLFUSamples = &samples
				}
				trace = []objiotracing.Event{
					{
						Op: objiotracing.WriteOp,
						Reason: objiotracing.ForCompaction,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 1,
						FileNum: 4,
						Offset: 1024 * 4,
						Size: 1024,
					},
					{
						Op: objiotracing.ReadOp,
						Reason: objiotracing.ForCompaction,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 1,
						FileNum: 4,
						Offset: 0,
						Size: 1024,
					},
					{
						Op: objiotracing.ReadOp,
						Reason: objiotracing.ForCompaction,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 1,
						FileNum: 4,
						Offset: 1024,
						Size: 1024,
					},
					{
						Op: objiotracing.RecordCacheHitOp,
						Reason: objiotracing.ForCompaction,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 1,
						FileNum: 4,
						Offset: 0,
						Size: 1024,
					},
					{
						Op: objiotracing.ReadOp,
						Reason: objiotracing.UnknownReason,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 6,
						FileNum: 4,
						Offset: 1024 * 4,
						Size: 1024,
					},
					{
						Op: objiotracing.ReadOp,
						Reason: objiotracing.UnknownReason,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 6,
						FileNum: 4,
						Offset: 1024 * 4,
						Size: 1024,
					},
					{
						Op: objiotracing.ReadOp,
						Reason: objiotracing.ForCompaction,
						BlockType: objiotracing.DataBlock,
						LevelPlusOne: 1,
						FileNum: 4,
						Offset: 0,
						Size: 1024,
					},
				}
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				require.Equal(t, 3, results.Hits)
				require.Equal(t, 3, results.Misses)
			})
			t.Run("only L5 & L6", func(t *testing.T) {
				config.L5AndL6Only = true
				defer func() {
					config.L5AndL6Only = false
				}()
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				// Reads to other levels don't count as either hits or misses.
				require.Equal(t, 1, results.Hits)
				require.Equal(t, 1, results.Misses)
			})
			t.Run("only user-facing reads", func(t *testing.T) {
				config.CacheUserFacingReadsOnly = true
				defer func() {
					config.CacheUserFacingReadsOnly = false
				}()
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				// Reads done for reasons other as part of compaction, etc. don't
				// count as either hits or misses.
				require.Equal(t, 1, results.Hits)
				require.Equal(t, 1, results.Misses)
			})
			t.Run("large block size", func(t *testing.T) {
				var blockSize int64 = 1024 * 10
				config.BlockSize = &blockSize
				defer func() {
					config.BlockSize = nil
				}()
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				// Initial miss will fill cache with what is needed for rest of reads
				// to be hits.
				require.Equal(t, 5, results.Hits)
				require.Equal(t, 1, results.Misses)
			})
			t.Run("write-thru", func(t *testing.T) {
				config.WriteThru = true
				defer func() {
					config.WriteThru = false
				}()
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				// Inclusion of write-thru leads to one more hit than first test case.
				require.Equal(t, 4, results.Hits)
				require.Equal(t, 2, results.Misses)
			})
			t.Run("write-thru & only user-facing reads", func(t *testing.T) {
				config.WriteThru = true
				config.CacheUserFacingReadsOnly = true
				defer func() {
					config.WriteThru = false
					config.CacheUserFacingReadsOnly = false
				}()
				results, err := Simulate(t.Name(), &wrappedTrace{inner: trace}, config)
				require.NoError(t, err)
				// Both reads that are (likely) user-facing are hits, since earlier
				// write done as part of compaction has filled cache.
				require.Equal(t, 2, results.Hits)
				require.Equal(t, 0, results.Misses)
			})
		})
	}
}
