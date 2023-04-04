package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/RaduBerinde/pebble_analysis/objiotracing/lib"
)

const port = 8089

type ListTracesResponse struct {
	Traces []string `json:"traces"`
}

// ListTraces returns all traces available in the traces/ directory.
func ListTraces() ListTracesResponse {
	traces, err := lib.ListTraces()
	checkErr(err, "reading traces directory")
	return ListTracesResponse{Traces: traces}
}

type PlotTraceRequest struct {
	Trace string `json:"trace"`
}

type PlotTraceResponse struct {
	Ticks            int `json:"ticks"`
	TickDurationSecs int `json:"tick_duration_secs"`

	TimeAxisUnixSecs []int `json:"time_axis_unix_secs"`

	ReadMBPS     []float64 `json:"read_mbps"`
	WriteMBPS    []float64 `json:"write_mbps"`
	CacheHitMBPS []float64 `json:"cache_hit_mbps"`

	ReadMBPSL5L6     []float64 `json:"read_mbps_l5_l6"`
	WriteMBPSL5L6    []float64 `json:"write_mbps_l5_l6"`
	CacheHitMBPSL5L6 []float64 `json:"cache_hit_mbps_l5_l6"`
}

func main() {
	http.HandleFunc("/listtraces", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		log.Printf("listtraces\n")
		res := ListTraces()
		buf, err := json.Marshal(&res)
		checkErr(err, "marshalling response")
		_, err = w.Write(buf)
		checkErr(err, "writing response")
	})

	fmt.Printf("Listening on :%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func checkErr(err error, context string) {
	if err != nil {
		log.Fatalf("error %s: %v", context, err)
	}
}
