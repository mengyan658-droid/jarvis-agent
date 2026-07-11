package service

import (
	"log/slog"
	"os"
	"strings"

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

func NewRuntime(cfg config.Config, logger *slog.Logger) *agent.Runtime {
	store := client.NewMockStore()
	jarvisClient := jarvis.NewMockClient(store)
	monitorClient := monitor.NewMockClient(store)
	changeClient := change.NewMockClient(store)
	cmdbClient := cmdb.NewMockClient(store)
	llmClient := newLLMClient(cfg, logger)

	tools := tool.NewRegistry(
		tool.QueryHostsTool{Client: jarvisClient},
		tool.GetHostTool{Client: jarvisClient},
		tool.QueryMetricsTool{Client: monitorClient},
		tool.QueryAlarmsTool{Client: monitorClient},
		tool.QueryChangesTool{Client: changeClient},
		tool.QueryCMDBTool{Client: cmdbClient},
		tool.ResolveTimeRangeTool{},
	).WithLogger(logger)
	if path := os.Getenv("TIME_TEST_LOG"); path != "" {
		tools.WithTimeTestLogPath(path)
	}
	workflows := workflow.NewRegistry(
		workflow.QueryFaultyHostsWorkflow{},
		workflow.DiagnoseHostWorkflow{},
		workflow.ToolLoopInvestigateHostWorkflow{},
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

func newLLMClient(cfg config.Config, logger *slog.Logger) client.LLMClient {
	provider := strings.ToLower(cfg.LLMProvider)
	switch provider {
	case "openai", "openai-compatible", "api":
		baseURL := cfg.LLMAPIBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		model := cfg.LLMModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		logger.Info("configured llm client", "provider", provider, "model", model, "base_url", baseURL)
		return llm.NewOpenAICompatibleClient(baseURL, cfg.LLMAPIKey, model).WithLogger(logger)
	case "glm", "zhipu", "bigmodel":
		baseURL := cfg.LLMAPIBaseURL
		if baseURL == "" {
			baseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
		model := cfg.LLMModel
		if model == "" {
			model = "glm-5.1"
		}
		logger.Info("configured llm client", "provider", provider, "model", model, "base_url", baseURL)
		return llm.NewOpenAICompatibleClient(baseURL, cfg.LLMAPIKey, model).WithLogger(logger)
	default:
		logger.Info("configured llm client", "provider", "mock")
		return llm.NewMockClient()
	}
}
