package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppPort           string
	AgentTimeout      time.Duration
	AgentMaxSteps     int
	AgentMaxToolCalls int
	LLMProvider       string
	LLMAPIBaseURL     string
	LLMAPIKey         string
	LLMModel          string
}

func Load() Config {
	llmProvider := getenv("LLM_PROVIDER", "mock")
	defaultAgentTimeout := 5 * time.Second
	if llmProvider != "mock" {
		defaultAgentTimeout = 30 * time.Second
	}
	return Config{
		AppPort:           getenv("APP_PORT", "8080"),
		AgentTimeout:      durationEnv("AGENT_TIMEOUT", defaultAgentTimeout),
		AgentMaxSteps:     intEnv("AGENT_MAX_STEPS", 10),
		AgentMaxToolCalls: intEnv("AGENT_MAX_TOOL_CALLS", 20),
		LLMProvider:       llmProvider,
		LLMAPIBaseURL:     getenv("LLM_API_BASE_URL", ""),
		LLMAPIKey:         getenv("LLM_API_KEY", ""),
		LLMModel:          getenv("LLM_MODEL", ""),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
