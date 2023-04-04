package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

const port = 8089

type ListTracesResponse struct {
	Traces []string `json:"traces"`
}

// ListTraces returns all traces available in the traces/ directory.
func ListTraces() ListTracesResponse {
	entries, err := os.ReadDir("traces")
	checkErr(err, "reading traces directory")
	var traces []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			traces = append(traces, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(traces)
	return ListTracesResponse{Traces: traces}
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
