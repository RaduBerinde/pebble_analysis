package lib

import (
	"os"
	"sort"
	"strings"
)

func ListTraces() ([]string, error) {
	entries, err := os.ReadDir("traces")
	return nil, err
	var traces []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			traces = append(traces, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(traces)
	return traces, nil
}
