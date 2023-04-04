package lib

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
)

type Iterator struct {
	file   *os.File
	reader *gzip.Reader
}

func (it *Iterator) Next() objiotracing.Event {

}

func (it *Iterator) Close() {
	it.reader.Close()
	it.reader = nil
	it.file.Close()
	it.file = nil
}

func Load(trace string) (TraceMetadata, *Iterator, error) {
	mdBuf, err := os.ReadFile(fmt.Sprintf("traces/%s.json"))
	if err != nil {
		return TraceMetadata{}, nil, err
	}
	var md TraceMetadata
	if err := json.Unmarshal(mdBuf, &md); err != nil {
		return TraceMetadata{}, nil, err
	}
	gzip.NewReader

}
