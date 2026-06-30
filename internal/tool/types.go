package tool

import (
	"context"
	"fmt"
	"time"
)

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
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Execute(ctx context.Context, name string, input any, recorder *Recorder) (any, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	started := time.Now()
	out, err := t.Execute(ctx, input)
	if recorder != nil {
		recorder.Record(name, started, err)
	}
	return out, err
}
