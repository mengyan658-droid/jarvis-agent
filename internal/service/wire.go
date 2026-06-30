package service

import (
	"jarvis-agent/internal/agent"
	"jarvis-agent/internal/client"
	"jarvis-agent/internal/client/change"
	"jarvis-agent/internal/client/cmdb"
	"jarvis-agent/internal/client/jarvis"
	"jarvis-agent/internal/client/llm"
	"jarvis-agent/internal/client/monitor"
	"jarvis-agent/internal/config"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
	"jarvis-agent/internal/workflow"
)

func NewRuntime(cfg config.Config) *agent.Runtime {
	store := client.NewMockStore()
	jarvisClient := jarvis.NewMockClient(store)
	monitorClient := monitor.NewMockClient(store)
	changeClient := change.NewMockClient(store)
	cmdbClient := cmdb.NewMockClient(store)
	llmClient := llm.NewMockClient()

	tools := tool.NewRegistry(
		tool.QueryHostsTool{Client: jarvisClient},
		tool.GetHostTool{Client: jarvisClient},
		tool.QueryMetricsTool{Client: monitorClient},
		tool.QueryAlarmsTool{Client: monitorClient},
		tool.QueryChangesTool{Client: changeClient},
		tool.QueryCMDBTool{Client: cmdbClient},
	)
	workflows := workflow.NewRegistry(
		workflow.QueryFaultyHostsWorkflow{},
		workflow.DiagnoseHostWorkflow{},
	)
	return &agent.Runtime{
		LLM:          llmClient,
		Tools:        tools,
		Workflows:    workflows,
		Analyzer:     domain.NewFaultAnalyzer(),
		Timeout:      cfg.AgentTimeout,
		MaxSteps:     cfg.AgentMaxSteps,
		MaxToolCalls: cfg.AgentMaxToolCalls,
	}
}
