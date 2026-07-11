package workflow

import (
	"context"
	"fmt"
	"testing"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/client/change"
	"jarvis-agent/internal/client/cmdb"
	"jarvis-agent/internal/client/jarvis"
	"jarvis-agent/internal/client/llm"
	"jarvis-agent/internal/client/monitor"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

func TestToolLoopInvestigateHostWorkflow(t *testing.T) {
	store := client.NewMockStore()
	tools := tool.NewRegistry(
		tool.GetHostTool{Client: jarvis.NewMockClient(store)},
		tool.QueryMetricsTool{Client: monitor.NewMockClient(store)},
		tool.QueryAlarmsTool{Client: monitor.NewMockClient(store)},
		tool.QueryChangesTool{Client: change.NewMockClient(store)},
		tool.QueryCMDBTool{Client: cmdb.NewMockClient(store)},
		tool.ResolveTimeRangeTool{},
	)
	result, err := ToolLoopInvestigateHostWorkflow{}.Run(context.Background(), Context{
		Intent:       client.Intent{Name: "tool_loop_investigate_host", Parameters: map[string]string{"host_id": "host-001"}},
		Tools:        tools,
		ToolRecorder: &tool.Recorder{},
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          llm.NewMockClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := result.Results.(ToolLoopInvestigationResult)
	if len(got.Trace) == 0 {
		t.Fatal("expected react trace")
	}
	if got.Assessment.HostID != "host-001" || !got.Assessment.IsFaulty {
		t.Fatalf("unexpected assessment: %+v", got.Assessment)
	}
}

func TestToolLoopSkipsDuplicateCanonicalToolCalls(t *testing.T) {
	store := client.NewMockStore()
	recorder := &tool.Recorder{}
	tools := tool.NewRegistry(
		tool.GetHostTool{Client: jarvis.NewMockClient(store)},
		tool.QueryMetricsTool{Client: monitor.NewMockClient(store)},
		tool.QueryAlarmsTool{Client: monitor.NewMockClient(store)},
		tool.QueryChangesTool{Client: change.NewMockClient(store)},
		tool.QueryCMDBTool{Client: cmdb.NewMockClient(store)},
		tool.ResolveTimeRangeTool{},
	)
	result, err := ToolLoopInvestigateHostWorkflow{}.Run(context.Background(), Context{
		Intent:       client.Intent{Name: "tool_loop_investigate_host", Parameters: map[string]string{"host_id": "host-001"}},
		Tools:        tools,
		ToolRecorder: recorder,
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          &duplicateToolCallLLM{},
		MaxSteps:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := result.Results.(ToolLoopInvestigationResult)
	duplicateSkipped := false
	for _, trace := range got.Trace {
		if trace.Function == tool.QueryMetricsToolName && trace.Status == "skipped_duplicate" {
			duplicateSkipped = true
		}
	}
	if !duplicateSkipped {
		t.Fatalf("expected duplicate query_metrics to be skipped: %+v", got.Trace)
	}
	metricsCalls := 0
	for _, call := range recorder.Calls() {
		if call.Name == tool.QueryMetricsToolName {
			metricsCalls++
		}
	}
	if metricsCalls != 1 {
		t.Fatalf("query_metrics should execute once, got %d calls: %+v", metricsCalls, recorder.Calls())
	}
}

type duplicateToolCallLLM struct {
	round int
}

func (l *duplicateToolCallLLM) ParseIntent(ctx context.Context, message string) (client.Intent, error) {
	return client.Intent{}, nil
}

func (l *duplicateToolCallLLM) GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error) {
	return "", nil
}

func (l *duplicateToolCallLLM) GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error) {
	return fmt.Sprintf("%s score=%d", assessment.HostID, assessment.Score), nil
}

func (l *duplicateToolCallLLM) ChatWithTools(ctx context.Context, messages []client.ToolChatMessage, tools []client.FunctionTool) (client.ToolChatMessage, error) {
	l.round++
	if l.round == 1 {
		return client.ToolChatMessage{
			Role: "assistant",
			ToolCalls: []client.ToolCall{
				testToolCall("call-1", tool.GetHostToolName, `{"host_id":"host-001"}`),
				testToolCall("call-2", tool.QueryMetricsToolName, `{"host_id":"host-001"}`),
				testToolCall("call-3", tool.QueryMetricsToolName, `{"host_id":"host-001","reason":"double check","include_raw":true}`),
				testToolCall("call-4", tool.QueryAlarmsToolName, `{"host_id":"host-001"}`),
				testToolCall("call-5", tool.QueryChangesToolName, `{"host_id":"host-001"}`),
				testToolCall("call-6", tool.QueryCMDBToolName, `{"host_id":"host-001"}`),
			},
		}, nil
	}
	return client.ToolChatMessage{
		Role:      "assistant",
		ToolCalls: []client.ToolCall{testToolCall("call-7", assessFaultFunctionName, `{"host_id":"host-001"}`)},
	}, nil
}

func testToolCall(id, name, arguments string) client.ToolCall {
	return client.ToolCall{
		ID:   id,
		Type: "function",
		Function: client.FunctionCall{
			Name:      name,
			Arguments: arguments,
		},
	}
}
