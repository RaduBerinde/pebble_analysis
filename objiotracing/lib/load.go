package lib

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"unsafe"

	gzip "github.com/klauspost/pgzip"

	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
)

// Iterator is used to stream Events from a compressed trace file.
type Iterator struct {
	file     *os.File
	gzReader *gzip.Reader
	buf      []byte
	done     bool
}

const eventSize = int(unsafe.Sizeof(objiotracing.Event{}))

func newIterator(file *os.File, gzReader *gzip.Reader) *Iterator {
	const bufSize = 1024
	return &Iterator{
		file:     file,
		gzReader: gzReader,
		buf:      make([]byte, bufSize*eventSize),
	}
}

// NextBatch returns a batch of events. If there are no more events, returns nil.
func (it *Iterator) NextBatch() ([]objiotracing.Event, error) {
	if it.done {
		return nil, nil
	}
	n, err := io.ReadFull(it.gzReader, it.buf)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			it.done = true
			if n < eventSize {
				return nil, nil
			}
		} else {
			return nil, err
		}

	}
	p := unsafe.Pointer(&it.buf[0])
	return unsafe.Slice((*objiotracing.Event)(p), n/eventSize), nil
}

func (it *Iterator) Close() {
	it.gzReader.Close()
	it.file.Close()
	*it = Iterator{}
}

// Load a trace; returns the metadata and a streaming iterator.
func Load(trace string) (TraceMetadata, *Iterator, error) {
	mdBuf, err := os.ReadFile(fmt.Sprintf("traces/%s.json", trace))
	if err != nil {
		return TraceMetadata{}, nil, err
	}
	var md TraceMetadata
	if err := json.Unmarshal(mdBuf, &md); err != nil {
		return TraceMetadata{}, nil, err
	}
	file, err := os.Open(fmt.Sprintf("traces/%s.gz", trace))
	if err != nil {
		return TraceMetadata{}, nil, err
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return TraceMetadata{}, nil, err
	}
	return md, newIterator(file, reader), nil
}
