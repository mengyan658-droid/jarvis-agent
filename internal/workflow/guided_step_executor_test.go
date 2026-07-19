package workflow

import (
	"context"
	"fmt"
	"testing"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

type guidedStepTestLLM struct{}

func (l guidedStepTestLLM) ParseIntent(ctx context.Context, message string) (client.Intent, error) {
	return client.Intent{}, nil
}

func (l guidedStepTestLLM) GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error) {
	return "", nil
}

func (l guidedStepTestLLM) GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error) {
	return "", nil
}

func (l guidedStepTestLLM) GenerateModelErrorDailyReport(ctx context.Context, facts any) (string, error) {
	return "", nil
}

func (l guidedStepTestLLM) ChatWithTools(ctx context.Context, messages []client.ToolChatMessage, tools []client.FunctionTool) (client.ToolChatMessage, error) {
	if len(messages) != 2 {
		return client.ToolChatMessage{}, fmt.Errorf("unexpected messages: %d", len(messages))
	}
	if len(tools) != 1 {
		return client.ToolChatMessage{}, fmt.Errorf("unexpected tools: %d", len(tools))
	}
	return client.ToolChatMessage{
		Role: "assistant",
		ToolCalls: []client.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: client.FunctionCall{
				Name:      tools[0].Function.Name,
				Arguments: `{"kind":"relative","amount":2,"unit":"hour"}`,
			},
		}},
	}, nil
}

func TestGuidedStepExecutorRunsStepsAndPlansToolCall(t *testing.T) {
	steps := []Step{}
	warnings := []string{}
	exec := NewGuidedStepExecutor(context.Background(), Context{
		Message: "查询最近2小时的数据",
		LLM:     guidedStepTestLLM{},
	}, &steps, &warnings)

	var planned tool.ResolveTimeRangeInput
	err := exec.Run(
		GuidedStep{Name: "plan_time", Run: func(exec *GuidedStepExecutor) error {
			call, err := exec.PlanToolCall(GuidedToolPlanRequest{
				SystemPrompt: "plan time",
				Tool:         tool.ResolveTimeRangeFunctionTool(),
			})
			if err != nil {
				return err
			}
			planned, err = DecodeGuidedToolArguments[tool.ResolveTimeRangeInput](call)
			return err
		}},
		GuidedStep{Name: "warn", Run: func(exec *GuidedStepExecutor) error {
			exec.Warn("fallback used")
			return nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if planned.Kind != "relative" || planned.Amount != 2 || planned.Unit != "hour" {
		t.Fatalf("unexpected plan: %+v", planned)
	}
	if len(steps) != 2 || steps[0].Name != "plan_time" || steps[1].Name != "warn" {
		t.Fatalf("unexpected steps: %+v", steps)
	}
	if len(warnings) != 1 || warnings[0] != "fallback used" {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
}

func TestGuidedStepExecutorExecutesTool(t *testing.T) {
	steps := []Step{}
	warnings := []string{}
	recorder := &tool.Recorder{}
	exec := NewGuidedStepExecutor(context.Background(), Context{
		Tools:        tool.NewRegistry(tool.ResolveTimeRangeTool{}),
		ToolRecorder: recorder,
	}, &steps, &warnings)

	err := exec.Run(GuidedStep{Name: "resolve_time", Run: func(exec *GuidedStepExecutor) error {
		out, err := exec.ExecuteTool(tool.ResolveTimeRangeToolName, tool.ResolveTimeRangeInput{Kind: "relative", Amount: 1, Unit: "hour"})
		if err != nil {
			return err
		}
		resolved := out.(domain.TimeRange)
		if resolved.Start.IsZero() || resolved.End.IsZero() {
			t.Fatalf("unexpected time range: %+v", resolved)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.Calls()) != 1 || recorder.Calls()[0].Name != tool.ResolveTimeRangeToolName {
		t.Fatalf("unexpected calls: %+v", recorder.Calls())
	}
}
