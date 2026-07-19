package workflow

import (
	"context"
	"fmt"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Result struct {
	Intent    string            `json:"intent"`
	Workflow  string            `json:"workflow"`
	Summary   string            `json:"summary"`
	Results   any               `json:"results"`
	Warnings  []string          `json:"warnings,omitempty"`
	Steps     []Step            `json:"execution_steps"`
	ToolCalls []tool.CallRecord `json:"tool_calls"`
}

type Context struct {
	Intent       client.Intent
	Message      string
	Tools        *tool.Registry
	ToolRecorder *tool.Recorder
	Analyzer     *domain.FaultAnalyzer
	LLM          client.LLMClient
	MaxSteps     int
}

type Workflow interface {
	Name() string
	Run(ctx context.Context, wfctx Context) (Result, error)
}

type Registry struct {
	workflows map[string]Workflow
}

func NewRegistry(workflows ...Workflow) *Registry {
	r := &Registry{workflows: map[string]Workflow{}}
	for _, wf := range workflows {
		r.Register(wf)
	}
	return r
}

func (r *Registry) Register(wf Workflow) {
	r.workflows[wf.Name()] = wf
}

func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.workflows[name]
	return ok
}

func (r *Registry) Get(name string) (Workflow, error) {
	wf, ok := r.workflows[name]
	if !ok {
		return nil, fmt.Errorf("workflow %q not found", name)
	}
	return wf, nil
}

func runStep(steps *[]Step, name string, fn func() error) error {
	started := time.Now()
	err := fn()
	step := Step{Name: name, Status: "ok", DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		step.Status = "error"
		step.Error = err.Error()
	}
	*steps = append(*steps, step)
	return err
}
