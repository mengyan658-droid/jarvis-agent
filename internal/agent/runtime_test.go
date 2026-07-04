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
