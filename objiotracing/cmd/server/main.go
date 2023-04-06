package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/RaduBerinde/pebble_analysis/objiotracing/lib"
	"github.com/cockroachdb/pebble/objstorage/objstorageprovider/objiotracing"
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
	NumTicks         int `json:"num_ticks"`
	TickDurationSecs int `json:"tick_duration_secs"`

	TimeAxisUnixSecs []int64 `json:"time_axis_unix_secs"`

	ReadMBPS     []float64 `json:"read_mbps"`
	WriteMBPS    []float64 `json:"write_mbps"`
	CacheHitMBPS []float64 `json:"cache_hit_mbps"`

	ReadMBPSL5L6     []float64 `json:"read_mbps_l5_l6"`
	WriteMBPSL5L6    []float64 `json:"write_mbps_l5_l6"`
	CacheHitMBPSL5L6 []float64 `json:"cache_hit_mbps_l5_l6"`
}


// TODO(josh): Produce a hit rate graph, to compare hit rate of productionized
// pebble block clock to simulated algorithms.
func Plot(req PlotTraceRequest) PlotTraceResponse {
	md, it, err := lib.Load(req.Trace)
	checkErr(err, fmt.Sprintf("loading trace %q", req.Trace))

	const targetTicks = 10000
	tickSecs := md.DurationSecs / targetTicks
	if tickSecs < 1 {
		tickSecs = 1
	}
	startTime, err := time.Parse(time.RFC3339, md.StartTime)
	checkErr(err, "parsing trace start time")
	var r PlotTraceResponse
	r.NumTicks = 1 + md.DurationSecs/tickSecs
	r.TickDurationSecs = tickSecs
	r.TimeAxisUnixSecs = make([]int64, 0, r.NumTicks)

	const (
		read = iota
		write
		hit
		readL56
		writeL56
		hitL56
	)
	metrics := [...]*[]float64{
		read:     &r.ReadMBPS,
		write:    &r.WriteMBPS,
		hit:      &r.CacheHitMBPS,
		readL56:  &r.ReadMBPSL5L6,
		writeL56: &r.WriteMBPSL5L6,
		hitL56:   &r.CacheHitMBPSL5L6,
	}
	curr := make([]float64, len(metrics))
	for i := range metrics {
		*metrics[i] = make([]float64, 0, r.NumTicks)
	}

	currentTick := startTime
	tickDuration := time.Second * time.Duration(tickSecs)
	toMBPS := 1.0 / (1024 * 1024) / float64(tickSecs)

	flush := func() {
		r.TimeAxisUnixSecs = append(r.TimeAxisUnixSecs, currentTick.Unix())
		for i := range metrics {
			*metrics[i] = append(*metrics[i], curr[i])
			curr[i] = 0
		}
	}
	for {
		events, err := it.NextBatch()
		checkErr(err, "iterating")
		if events == nil {
			break
		}
		for i := range events {
			ev := &events[i]
			t := time.Unix(0, ev.StartUnixNano)
			for t.Sub(currentTick) >= tickDuration {
				flush()
				currentTick = currentTick.Add(tickDuration)
			}
			isL56 := ev.LevelPlusOne > 5
			size := float64(ev.Size) * toMBPS
			switch ev.Op {
			case objiotracing.ReadOp:
				curr[read] += size
				if isL56 {
					curr[readL56] += size
				}
			case objiotracing.WriteOp:
				curr[write] += size
				if isL56 {
					curr[writeL56] += size
				}
			case objiotracing.RecordCacheHitOp:
				curr[hit] += size
				if isL56 {
					curr[hitL56] += size
				}
			}
		}
	}
	flush()

	return r
}

type SimulateTraceRequest struct {
	Trace string `json:"trace"`
}

// TODO(josh): Consider returning a set of points to graph instead.
type SimulateTraceResponse struct {
	CacheSize []int `json:"cache_size"`
	Results   []ResultsPerReplacementPolicy `json:"results_per_replacement_policy"`
}

type ResultsPerReplacementPolicy struct {
	ReplacementPolicy string `json:"replacement_policy"`
	Results           []ResultsPerOptionSet `json:"results_per_option_set"`
}

type ResultsPerOptionSet struct {
	OptionSet string `json:"option_set"`
	HitRate   []float64 `json:"hit_rate"`
}

var configs = []lib.Config{
	{},
	{
		WriteThru: true,
	},
	{
		L5AndL6Only: true,
	},
	{
		CacheUserFacingReadsOnly: true,
	},
	{
		WriteThru: true,
		L5AndL6Only: true,
		CacheUserFacingReadsOnly: true,
	},
}

// TODO(josh): Enable varying block size.
// TODO(josh): Enable measure hit rate over time.
func Simulate(req SimulateTraceRequest) SimulateTraceResponse {
	const (
		start = 1024      // 1K
		end = 1024 * 1000 // 1MB
		targetTicks = 10
		increment = (end - start) / targetTicks
	)

	resp := SimulateTraceResponse{}
	for cacheSize := start; cacheSize < end; cacheSize += increment {
		resp.CacheSize = append(resp.CacheSize, cacheSize)
	}
	for i, policy := range []lib.ReplacementPolicy{lib.TinyLFU, lib.ClockPro, lib.S4LRU} {
		resp.Results = append(resp.Results, ResultsPerReplacementPolicy{
			ReplacementPolicy: policy.String(),
		})
		for j, config := range configs {
			config.Policy = policy
			resp.Results[i].Results = append(resp.Results[i].Results, ResultsPerOptionSet{
				OptionSet: config.String(),
			})
			for cacheSize := start; cacheSize < end; cacheSize += increment {
				config.CacheSize = cacheSize
				if policy == lib.TinyLFU {
					config.TinyLFUSamples = 10 * config.CacheSize
				}
				func() {
					log.Printf("simulate %s / %v / %s / %s\n", req.Trace, cacheSize, policy.String(), config.String())

					_, it, err := lib.Load(req.Trace)
					checkErr(err, fmt.Sprintf("loading trace %q", req.Trace))
					defer it.Close()

					results, err := lib.Simulate(req.Trace, it, config)
					checkErr(err, fmt.Sprintf("calling simulate %q", req.Trace))
					hitRate := float64(results.Hits) / float64(results.Hits + results.Misses)

					resp.Results[i].Results[j].HitRate = append(resp.Results[i].Results[j].HitRate, hitRate)
				}()
			}
		}
	}

	return resp
}

func main() {
	http.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("list\n")
		w.Header().Add("Access-Control-Allow-Origin", "*")

		res := ListTraces()
		buf, err := json.Marshal(&res)
		checkErr(err, "marshalling response")
		_, _ = w.Write(buf)
	})

	http.HandleFunc("/plot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		reqBuf, err := io.ReadAll(r.Body)
		checkErr(err, "reading body")
		var req PlotTraceRequest
		checkErr(json.Unmarshal(reqBuf, &req), "unmarshalling request")
		log.Printf("plot %s\n", req.Trace)
		res := Plot(req)

		respBuf, err := json.Marshal(&res)
		checkErr(err, "marshalling response")
		_, _ = w.Write(respBuf)
	})

	http.HandleFunc("/simulate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		reqBuf, err := io.ReadAll(r.Body)
		checkErr(err, "reading body")
		var req SimulateTraceRequest
		checkErr(json.Unmarshal(reqBuf, &req), "unmarshalling request")
		log.Printf("simulate %s\n", req.Trace)
		res := Simulate(req)

		respBuf, err := json.Marshal(&res)
		checkErr(err, "marshalling response")
		_, _ = w.Write(respBuf)
	})

	fmt.Printf("Listening on :%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func checkErr(err error, context string) {
	if err != nil {
		log.Fatalf("error %s: %v", context, err)
	}
}
