package client

import (
	"context"
	"encoding/json"

	"jarvis-agent/internal/domain"
)

type HostQuery struct {
	Region      string
	Environment string
}

type JarvisClient interface {
	QueryHosts(ctx context.Context, query HostQuery) ([]domain.Host, error)
	GetHost(ctx context.Context, hostID string) (domain.Host, error)
}

type MonitorClient interface {
	QueryHostMetrics(ctx context.Context, hostID string) (domain.HostMetrics, error)
	QueryActiveAlarms(ctx context.Context, hostID string) ([]domain.Alarm, error)
}

type ChangeClient interface {
	QueryRecentChanges(ctx context.Context, hostID string, timeRange domain.TimeRange) ([]domain.ChangeRecord, error)
}

type CMDBClient interface {
	QueryHostMetadata(ctx context.Context, hostID string) (map[string]string, error)
}

type Intent struct {
	Name       string            `json:"name"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type LLMClient interface {
	ParseIntent(ctx context.Context, message string) (Intent, error)
	GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error)
	GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error)
}

type FunctionCallingClient interface {
	ChatWithTools(ctx context.Context, messages []ToolChatMessage, tools []FunctionTool) (ToolChatMessage, error)
}

type ToolChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type FunctionTool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func MarshalToolResult(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"error":"marshal tool result failed"}`
	}
	return string(data)
}
