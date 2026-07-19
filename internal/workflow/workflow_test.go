package workflow

import (
	"context"
	"strings"
	"testing"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/client/change"
	"jarvis-agent/internal/client/cmdb"
	"jarvis-agent/internal/client/jarvis"
	"jarvis-agent/internal/client/llm"
	"jarvis-agent/internal/client/monitor"
	"jarvis-agent/internal/client/requestcount"
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

func TestModelErrorDailyReportWorkflow(t *testing.T) {
	store := client.NewMockStore()
	tools := tool.NewRegistry(
		tool.QueryErrorRequestCountsTool{Client: requestcount.NewMockClient(store)},
		tool.ResolveTimeRangeTool{},
	)
	result, err := ModelErrorDailyReportWorkflow{}.Run(context.Background(), Context{
		Intent: client.Intent{Name: ModelErrorDailyReportWorkflowName, Parameters: map[string]string{
			"device_models":     "iphone-15",
			"error_code":        "E500",
			"since":             "24h",
			"aggregation_value": "1",
			"aggregation_unit":  "h",
		}},
		Tools:        tools,
		ToolRecorder: &tool.Recorder{},
		Analyzer:     domain.NewFaultAnalyzer(),
		LLM:          llm.NewMockClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	report := result.Results.(ModelErrorDailyReportResult)
	if report.TotalCount != 83 {
		t.Fatalf("unexpected total: %+v", report)
	}
	if len(report.ByDeviceModel) != 1 || report.ByDeviceModel[0].Name != "iphone-15" || report.ByDeviceModel[0].Count != 83 {
		t.Fatalf("unexpected model dimension: %+v", report.ByDeviceModel)
	}
	if len(report.ByTime) == 0 {
		t.Fatalf("expected time dimension: %+v", report)
	}
	if result.Summary == "" || report.ReportMarkdown == "" {
		t.Fatal("expected markdown report")
	}
	if result.Summary != report.ReportMarkdown {
		t.Fatal("summary should contain markdown report")
	}
	if !strings.Contains(report.ReportMarkdown, "# 机型错误码数量日报") || !strings.Contains(report.ReportMarkdown, "## 时间趋势分析") {
		t.Fatalf("unexpected markdown report: %s", report.ReportMarkdown)
	}
}
