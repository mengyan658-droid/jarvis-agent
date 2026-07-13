package agent_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"jarvis-agent/internal/config"
	"jarvis-agent/internal/service"
	"jarvis-agent/internal/workflow"
)

func TestRuntimeFallbacksUnknownIntentToToolLoop(t *testing.T) {
	runtime := service.NewRuntime(config.Config{
		AgentTimeout:      5 * time.Second,
		AgentMaxSteps:     10,
		AgentMaxToolCalls: 20,
	}, slog.Default())

	result, err := runtime.Query(context.Background(), "req-test", "帮我看看 host-001")
	if err != nil {
		t.Fatal(err)
	}
	if result.Workflow != workflow.ToolLoopInvestigateHostWorkflowName {
		t.Fatalf("unexpected workflow: %s", result.Workflow)
	}
	if result.Intent != workflow.ToolLoopInvestigateHostWorkflowName {
		t.Fatalf("unexpected intent: %s", result.Intent)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected fallback warning")
	}
}

func TestRuntimeRoutesWithSkillSelection(t *testing.T) {
	runtime := service.NewRuntime(config.Config{
		AgentTimeout:      5 * time.Second,
		AgentMaxSteps:     10,
		AgentMaxToolCalls: 20,
	}, slog.Default())

	result, err := runtime.Query(context.Background(), "req-test", "诊断 host-003 最近2天的问题")
	if err != nil {
		t.Fatal(err)
	}
	if result.Workflow != "diagnose_host" {
		t.Fatalf("unexpected workflow: %s", result.Workflow)
	}
	if result.Intent != "diagnose_host" {
		t.Fatalf("unexpected intent: %s", result.Intent)
	}
	for _, warning := range result.Warnings {
		if warning == "skill router returned no skill; used intent parser" {
			t.Fatalf("skill router should select a skill, warnings=%+v", result.Warnings)
		}
	}
}
