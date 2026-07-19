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
	"jarvis-agent/internal/client/requestcount"
	"jarvis-agent/internal/config"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/skill"
	"jarvis-agent/internal/tool"
	"jarvis-agent/internal/workflow"
)

func NewRuntime(cfg config.Config, logger *slog.Logger) *agent.Runtime {
	store := client.NewMockStore()
	jarvisClient := jarvis.NewMockClient(store)
	monitorClient := monitor.NewMockClient(store)
	changeClient := change.NewMockClient(store)
	cmdbClient := cmdb.NewMockClient(store)
	requestCountClient := requestcount.NewMockClient(store)
	llmClient := newLLMClient(cfg, logger)

	tools := tool.NewRegistry(
		tool.QueryHostsTool{Client: jarvisClient},
		tool.GetHostTool{Client: jarvisClient},
		tool.QueryMetricsTool{Client: monitorClient},
		tool.QueryAlarmsTool{Client: monitorClient},
		tool.QueryChangesTool{Client: changeClient},
		tool.QueryCMDBTool{Client: cmdbClient},
		tool.QueryErrorRequestCountsTool{Client: requestCountClient},
		tool.ResolveTimeRangeTool{},
	).WithLogger(logger)
	if path := os.Getenv("TIME_TEST_LOG"); path != "" {
		tools.WithTimeTestLogPath(path)
	}
	workflows := workflow.NewRegistry(
		workflow.QueryFaultyHostsWorkflow{},
		workflow.DiagnoseHostWorkflow{},
		workflow.ToolLoopInvestigateHostWorkflow{},
		workflow.ModelErrorDailyReportWorkflow{},
	)
	skills := loadSkills(logger)
	if err := skills.ValidateTools(tools.Has); err != nil {
		logger.Warn("skill tool validation failed", "error", err)
	}
	if err := skills.ValidateExecutionTargets(workflows.Has); err != nil {
		logger.Warn("skill execution target validation failed", "error", err)
	}
	return &agent.Runtime{
		LLM:          llmClient,
		Tools:        tools,
		Skills:       skills,
		Workflows:    workflows,
		Analyzer:     domain.NewFaultAnalyzer(),
		Timeout:      cfg.AgentTimeout,
		MaxSteps:     cfg.AgentMaxSteps,
		MaxToolCalls: cfg.AgentMaxToolCalls,
	}
}

func loadSkills(logger *slog.Logger) *skill.Registry {
	dirs := []string{}
	if dir := os.Getenv("SKILLS_DIR"); dir != "" {
		dirs = append(dirs, dir)
	}
	dirs = append(dirs, "skills", "../skills", "../../skills")
	for _, dir := range dirs {
		registry, err := skill.LoadDir(dir)
		if err != nil {
			logger.Warn("load skills failed", "dir", dir, "error", err)
			continue
		}
		if len(registry.Names()) == 0 {
			continue
		}
		logger.Info("loaded skills", "dir", dir, "count", len(registry.Names()), "skills", registry.Names())
		return registry
	}
	logger.Warn("no skills loaded")
	registry, _ := skill.NewRegistry()
	return registry
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
