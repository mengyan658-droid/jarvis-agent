package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type requestIDContextKey struct{}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

type CallRecord struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Recorder struct {
	calls []CallRecord
}

func (r *Recorder) Record(name string, started time.Time, err error) {
	record := CallRecord{Name: name, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		record.Error = err.Error()
	}
	r.calls = append(r.calls, record)
}

func (r *Recorder) Calls() []CallRecord {
	return append([]CallRecord(nil), r.calls...)
}

type Tool interface {
	Name() string
	Execute(ctx context.Context, input any) (any, error)
}

type Registry struct {
	tools           map[string]Tool
	logger          *slog.Logger
	timeTestLogPath string
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

func (r *Registry) WithLogger(logger *slog.Logger) *Registry {
	r.logger = logger
	return r
}

func (r *Registry) WithTimeTestLogPath(path string) *Registry {
	r.timeTestLogPath = path
	return r
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.tools[name]
	return ok
}

func (r *Registry) Execute(ctx context.Context, name string, input any, recorder *Recorder) (any, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	started := time.Now()
	if r.logger != nil {
		r.logger.Info("tool call started",
			"request_id", requestIDFromContext(ctx),
			"tool", name,
			"input", loggableToolInput(input),
		)
	}
	if name == QueryChangesToolName {
		if err := r.writeTimeTestLog(ctx, input); err != nil && r.logger != nil {
			r.logger.Warn("write time test log failed", "request_id", requestIDFromContext(ctx), "error", err)
		}
	}
	out, err := t.Execute(ctx, input)
	if recorder != nil {
		recorder.Record(name, started, err)
	}
	if r.logger != nil {
		attrs := []any{
			"request_id", requestIDFromContext(ctx),
			"tool", name,
			"duration_ms", time.Since(started).Milliseconds(),
		}
		if err != nil {
			attrs = append(attrs, "error", err.Error())
		}
		r.logger.Info("tool call finished", attrs...)
	}
	return out, err
}

func (r *Registry) writeTimeTestLog(ctx context.Context, input any) error {
	if r.timeTestLogPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.timeTestLogPath), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(r.timeTestLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(map[string]any{
		"ts":         time.Now().Format(time.RFC3339Nano),
		"msg":        "query_changes time test",
		"request_id": requestIDFromContext(ctx),
		"tool":       QueryChangesToolName,
		"input":      loggableToolInput(input),
	})
}

func loggableToolInput(input any) any {
	switch v := input.(type) {
	case QueryChangesInput:
		return map[string]any{
			"host_id": v.HostID,
			"time_range": map[string]any{
				"range_id":       v.TimeRange.RangeID,
				"source":         v.TimeRange.Source,
				"timezone":       v.TimeRange.Timezone,
				"interval":       v.TimeRange.Interval,
				"duration_sec":   v.TimeRange.DurationSec,
				"start_time":     v.TimeRange.Start.Format(time.RFC3339Nano),
				"end_time":       v.TimeRange.End.Format(time.RFC3339Nano),
				"start_time_sec": v.TimeRange.StartUnixSec,
				"end_time_sec":   v.TimeRange.EndUnixSec,
				"start_time_ms":  v.TimeRange.StartUnixMS,
				"end_time_ms":    v.TimeRange.EndUnixMS,
			},
		}
	case QueryErrorRequestCountsInput:
		return map[string]any{
			"device_models": v.DeviceModels,
			"idcs":          v.IDCs,
			"error_code":    v.ErrorCode,
			"aggregation": map[string]any{
				"value": v.Aggregation.Value,
				"unit":  v.Aggregation.Unit,
			},
			"time_range": map[string]any{
				"range_id":       v.TimeRange.RangeID,
				"source":         v.TimeRange.Source,
				"timezone":       v.TimeRange.Timezone,
				"interval":       v.TimeRange.Interval,
				"duration_sec":   v.TimeRange.DurationSec,
				"start_time":     v.TimeRange.Start.Format(time.RFC3339Nano),
				"end_time":       v.TimeRange.End.Format(time.RFC3339Nano),
				"start_time_sec": v.TimeRange.StartUnixSec,
				"end_time_sec":   v.TimeRange.EndUnixSec,
				"start_time_ms":  v.TimeRange.StartUnixMS,
				"end_time_ms":    v.TimeRange.EndUnixMS,
			},
		}
	case ResolveTimeRangeInput:
		return map[string]any{
			"kind":       v.Kind,
			"amount":     v.Amount,
			"unit":       v.Unit,
			"start_text": v.StartText,
			"end_text":   v.EndText,
			"timezone":   v.Timezone,
		}
	default:
		return input
	}
}
