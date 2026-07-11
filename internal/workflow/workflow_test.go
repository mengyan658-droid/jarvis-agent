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

func TestQueryFaultyHostsWorkflow(t *testing.T) {
	store := client.NewMockStore()
	tools := tool.NewRegistry(
		tool.QueryHostsTool{Client: jarvis.NewMockClient(store)},
		tool.GetHostTool{Client: jarvis.NewMockClient(store)},
		tool.QueryMetricsTool{Client: monitor.NewMockClient(store)},
		tool.QueryAlarmsTool{Client: monitor.NewMockClient(store)},
		tool.QueryChangesTool{Client: change.NewMockClient(store)},
		tool.QueryCMDBTool{Client: cmdb.NewMockClient(store)},
		tool.ResolveTimeRangeTool{},
	)
	result, err := QueryFaultyHostsWorkflow{}.Run(context.Background(), Context{
		Intent:       client.Intent{Name: "query_faulty_hosts", Parameters: map[string]string{"region": "east-china", "environment": "production"}},
		Tools:        tools,
		ToolRecorder: &tool.Recorder{},
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          llm.NewMockClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assessments := result.Results.([]domain.FaultAssessment)
	if len(assessments) != 1 || assessments[0].HostID != "host-001" {
		t.Fatalf("unexpected assessments: %+v", assessments)
	}
}

func TestDiagnoseHostWorkflow(t *testing.T) {
	store := client.NewMockStore()
	tools := tool.NewRegistry(
		tool.GetHostTool{Client: jarvis.NewMockClient(store)},
		tool.QueryMetricsTool{Client: monitor.NewMockClient(store)},
		tool.QueryAlarmsTool{Client: monitor.NewMockClient(store)},
		tool.QueryChangesTool{Client: change.NewMockClient(store)},
		tool.QueryCMDBTool{Client: cmdb.NewMockClient(store)},
		tool.ResolveTimeRangeTool{},
	)
	result, err := DiagnoseHostWorkflow{}.Run(context.Background(), Context{
		Intent:       client.Intent{Name: "diagnose_host", Parameters: map[string]string{"host_id": "host-003"}},
		Tools:        tools,
		ToolRecorder: &tool.Recorder{},
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          llm.NewMockClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment := result.Results.(domain.FaultAssessment)
	if assessment.Level != domain.FaultLevelCritical {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
}
