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
	if result.Skill != "diagnose_host" {
		t.Fatalf("unexpected skill: %s", result.Skill)
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

func TestRuntimeExecutesToolLoopSkill(t *testing.T) {
	runtime := service.NewRuntime(config.Config{
		AgentTimeout:      5 * time.Second,
		AgentMaxSteps:     10,
		AgentMaxToolCalls: 20,
	}, slog.Default())

	result, err := runtime.Query(context.Background(), "req-test", "排查 host-001 的根因")
	if err != nil {
		t.Fatal(err)
	}
	if result.Skill != workflow.ToolLoopInvestigateHostWorkflowName {
		t.Fatalf("unexpected skill: %s", result.Skill)
	}
	if result.Workflow != workflow.ToolLoopInvestigateHostWorkflowName {
		t.Fatalf("unexpected workflow: %s", result.Workflow)
	}
	for _, warning := range result.Warnings {
		if warning == "intent is unknown; routed to tool loop workflow" {
			t.Fatalf("tool loop skill should not use unknown fallback, warnings=%+v", result.Warnings)
		}
	}
}

func TestRuntimeRoutesModelErrorDailyReportSkill(t *testing.T) {
	runtime := service.NewRuntime(config.Config{
		AgentTimeout:      5 * time.Second,
		AgentMaxSteps:     10,
		AgentMaxToolCalls: 20,
	}, slog.Default())

	result, err := runtime.Query(context.Background(), "req-test", "生成最近24小时 iphone-15 错误码 E500 的数量日报")
	if err != nil {
		t.Fatal(err)
	}
	if result.Skill != "model_error_daily_report" {
		t.Fatalf("unexpected skill: %s", result.Skill)
	}
	if result.Workflow != "model_error_daily_report" {
		t.Fatalf("unexpected workflow: %s", result.Workflow)
	}
	if result.Summary == "" {
		t.Fatal("expected summary")
	}
}
