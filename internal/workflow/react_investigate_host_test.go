package workflow

import (
	"context"
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

func TestReactInvestigateHostWorkflow(t *testing.T) {
	store := client.NewMockStore()
	tools := tool.NewRegistry(
		tool.GetHostTool{Client: jarvis.NewMockClient(store)},
		tool.QueryMetricsTool{Client: monitor.NewMockClient(store)},
		tool.QueryAlarmsTool{Client: monitor.NewMockClient(store)},
		tool.QueryChangesTool{Client: change.NewMockClient(store)},
		tool.QueryCMDBTool{Client: cmdb.NewMockClient(store)},
	)
	result, err := ReactInvestigateHostWorkflow{}.Run(context.Background(), Context{
		Intent:       client.Intent{Name: "react_investigate_host", Parameters: map[string]string{"host_id": "host-001"}},
		Tools:        tools,
		ToolRecorder: &tool.Recorder{},
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          llm.NewMockClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := result.Results.(ReactInvestigationResult)
	if len(got.Trace) == 0 {
		t.Fatal("expected react trace")
	}
	if got.Assessment.HostID != "host-001" || !got.Assessment.IsFaulty {
		t.Fatalf("unexpected assessment: %+v", got.Assessment)
	}
}
