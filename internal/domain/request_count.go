package domain

import (
	"fmt"
	"strings"
	"time"
)

type TimeAggregation struct {
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

func (a TimeAggregation) Duration() (time.Duration, error) {
	if a.Value <= 0 {
		return 0, fmt.Errorf("aggregation value must be positive")
	}
	switch strings.ToLower(strings.TrimSpace(a.Unit)) {
	case "m", "min", "minute", "minutes":
		return time.Duration(a.Value) * time.Minute, nil
	case "h", "hour", "hours":
		return time.Duration(a.Value) * time.Hour, nil
	case "d", "day", "days":
		return time.Duration(a.Value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported aggregation unit %q", a.Unit)
	}
}

type ErrorRequestCountSample struct {
	Timestamp   time.Time `json:"timestamp"`
	DeviceModel string    `json:"device_model"`
	IDC         string    `json:"idc"`
	ErrorCode   string    `json:"error_code"`
	Count       int       `json:"count"`
}

type ErrorRequestCount struct {
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	DeviceModel string    `json:"device_model"`
	IDC         string    `json:"idc"`
	ErrorCode   string    `json:"error_code"`
	Count       int       `json:"count"`
}
