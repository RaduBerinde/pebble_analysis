package lib

type TraceMetadata struct {
	Name         string `json:"name"`
	StartTime    string `json:"start_time"`
	DurationSecs int    `json:"duration_secs"`
}
