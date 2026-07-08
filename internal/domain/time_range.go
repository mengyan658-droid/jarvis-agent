package domain

import (
	"fmt"
	"time"
)

type TimeRange struct {
	RangeID      string    `json:"range_id"`
	Start        time.Time `json:"start_time"`
	End          time.Time `json:"end_time"`
	Now          time.Time `json:"now"`
	Timezone     string    `json:"timezone"`
	Source       string    `json:"source"`
	Interval     string    `json:"interval"`
	DurationMS   int64     `json:"duration_ms"`
	DurationSec  int64     `json:"duration_sec"`
	StartUnixSec int64     `json:"start_time_sec"`
	EndUnixSec   int64     `json:"end_time_sec"`
	NowUnixSec   int64     `json:"now_sec"`
	StartUnixMS  int64     `json:"start_time_ms"`
	EndUnixMS    int64     `json:"end_time_ms"`
	NowUnixMS    int64     `json:"now_ms"`
	IsDefault    bool      `json:"is_default"`
}

func NewTimeRange(start, end, now time.Time, timezone, source string, isDefault bool) TimeRange {
	duration := end.Sub(start)
	return TimeRange{
		RangeID:      fmt.Sprintf("time_range:%s:%d:%d", source, start.UnixMilli(), end.UnixMilli()),
		Start:        start,
		End:          end,
		Now:          now,
		Timezone:     timezone,
		Source:       source,
		Interval:     "[start,end)",
		DurationMS:   duration.Milliseconds(),
		DurationSec:  int64(duration.Seconds()),
		StartUnixSec: start.Unix(),
		EndUnixSec:   end.Unix(),
		NowUnixSec:   now.Unix(),
		StartUnixMS:  start.UnixMilli(),
		EndUnixMS:    end.UnixMilli(),
		NowUnixMS:    now.UnixMilli(),
		IsDefault:    isDefault,
	}
}
