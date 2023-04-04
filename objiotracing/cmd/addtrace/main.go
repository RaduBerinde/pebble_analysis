package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
	"unsafe"

	"github.com/RaduBerinde/pebble_analysis/objiotracing/lib"
	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
)

type Event = objiotracing.Event

const eventSize = int(unsafe.Sizeof(Event{}))

func main() {
	if len(os.Args) < 2 {
		checkErr(errors.New("usage: addtrace <trace-name> <trace-files>..."))
		os.Exit(1)
	}
	traceName := os.Args[1]
	filenames := os.Args[2:]
	fmt.Printf("Creating trace %q\n", traceName)
	var size int64
	for _, name := range filenames {
		info, err := os.Stat(name)
		checkErr(err)
		size += info.Size()
	}

	buf := bytes.NewBuffer(make([]byte, 0, int(size)))
	for _, name := range filenames {
		fmt.Printf("Reading %s..\n", name)
		f, err := os.Open(name)
		checkErr(err)
		_, err = io.Copy(buf, f)
		checkErr(err)
		checkErr(f.Close())
	}

	asBytes := buf.Bytes()
	// Should be a no-op, but just in case.
	asBytes = asBytes[:len(asBytes)/eventSize*eventSize]
	if len(asBytes) == 0 {
		checkErr(errors.New("no traces"))
	}
	p := unsafe.Pointer(&asBytes[0])
	events := unsafe.Slice((*Event)(p), len(asBytes)/eventSize)

	fmt.Printf("Sorting %d events..\n", len(events))
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartUnixNano < events[j].StartUnixNano
	})

	var md lib.TraceMetadata
	md.Name = traceName
	startTime := time.Unix(0, events[0].StartUnixNano)
	endTime := time.Unix(0, events[len(events)-1].StartUnixNano)
	md.StartTime = startTime.Format(time.RFC3339)
	md.DurationSecs = int((endTime.Sub(startTime) + time.Second - 1) / time.Second)
	md.NumEvents = len(events)

	outFilename := fmt.Sprintf("traces/%s.gz", traceName)
	fmt.Printf("Writing %s..\n", outFilename)
	out, err := os.Create(outFilename)
	checkErr(err)
	w := gzip.NewWriter(out)
	_, err = w.Write(asBytes)
	checkErr(err)
	checkErr(w.Close())
	checkErr(out.Close())

	jsonBuf, err := json.Marshal(&md)
	checkErr(err)
	checkErr(os.WriteFile(fmt.Sprintf("traces/%s.json", traceName), jsonBuf, 0666))
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
