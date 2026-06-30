package agent

import (
	"context"
	"errors"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
	"jarvis-agent/internal/workflow"
)

type Runtime struct {
	LLM          client.LLMClient
	Tools        *tool.Registry
	Workflows    *workflow.Registry
	Analyzer     *domain.FaultAnalyzer
	Timeout      time.Duration
	MaxSteps     int
	MaxToolCalls int
}

type QueryResult struct {
	RequestID  string            `json:"request_id"`
	Intent     string            `json:"intent"`
	Workflow   string            `json:"workflow"`
	Summary    string            `json:"summary"`
	Results    any               `json:"results"`
	Warnings   []string          `json:"warnings,omitempty"`
	Steps      []workflow.Step   `json:"execution_steps"`
	ToolCalls  []tool.CallRecord `json:"tool_calls"`
	DurationMS int64             `json:"duration_ms"`
}

func (r *Runtime) Query(ctx context.Context, requestID, message string) (QueryResult, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	intent, err := r.LLM.ParseIntent(ctx, message)
	if err != nil {
		return QueryResult{}, err
	}
	if intent.Name == "" || intent.Name == "unknown" {
		return QueryResult{}, errors.New("unsupported intent")
	}
	wf, err := r.Workflows.Get(intent.Name)
	if err != nil {
		return QueryResult{}, err
	}
	recorder := &tool.Recorder{}
	result, err := wf.Run(ctx, workflow.Context{
		Intent:       intent,
		Tools:        r.Tools,
		ToolRecorder: recorder,
		Analyzer:     r.Analyzer,
		LLM:          r.LLM,
		MaxSteps:     r.MaxSteps,
	})
	if err != nil {
		return QueryResult{}, err
	}
	if r.MaxToolCalls > 0 && len(result.ToolCalls) > r.MaxToolCalls {
		result.Warnings = append(result.Warnings, "tool call count exceeded configured limit")
	}
	return QueryResult{
		RequestID:  requestID,
		Intent:     result.Intent,
		Workflow:   result.Workflow,
		Summary:    result.Summary,
		Results:    result.Results,
		Warnings:   result.Warnings,
		Steps:      result.Steps,
		ToolCalls:  result.ToolCalls,
		DurationMS: time.Since(started).Milliseconds(),
	}, nil
}
