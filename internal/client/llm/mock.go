package llm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
)

type MockClient struct {
	Behavior client.MockBehavior
}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (c *MockClient) ParseIntent(ctx context.Context, message string) (client.Intent, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return client.Intent{}, err
	}
	params := map[string]string{}
	name := "unknown"
	if strings.Contains(message, "故障机") || strings.Contains(message, "异常机器") {
		name = "query_faulty_hosts"
	}
	if strings.Contains(message, "排查") || strings.Contains(message, "根因") {
		if id := extractHostID(message); id != "" {
			name = "tool_loop_investigate_host"
			params["host_id"] = id
		}
	}
	if strings.Contains(message, "诊断") || strings.Contains(message, "分析") {
		if id := extractHostID(message); id != "" {
			name = "diagnose_host"
			params["host_id"] = id
		}
	}
	if strings.Contains(message, "华东") {
		params["region"] = "east-china"
	}
	if strings.Contains(message, "华北") {
		params["region"] = "north-china"
	}
	if strings.Contains(message, "华南") {
		params["region"] = "south-china"
	}
	if strings.Contains(message, "生产") {
		params["environment"] = "production"
	}
	if strings.Contains(message, "预发") || strings.Contains(message, "测试") {
		params["environment"] = "staging"
	}
	if strings.Contains(message, "最近一小时") {
		params["since"] = "1h"
	}
	if id := extractHostID(message); id != "" {
		params["host_id"] = id
	}
	return client.Intent{Name: name, Parameters: params}, nil
}

func (c *MockClient) GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("共发现 %d 台故障机器，已按故障评分从高到低排序。", len(assessments)), nil
}

func (c *MockClient) GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s 评分 %d，等级 %s，故障状态为 %t。", assessment.HostID, assessment.Score, assessment.Level, assessment.IsFaulty), nil
}

func (c *MockClient) ChatWithTools(ctx context.Context, messages []client.ToolChatMessage, tools []client.FunctionTool) (client.ToolChatMessage, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return client.ToolChatMessage{}, err
	}
	hostID := "host-001"
	for _, msg := range messages {
		if id := extractHostID(msg.Content); id != "" {
			hostID = id
		}
	}
	toolResults := 0
	for _, msg := range messages {
		if msg.Role == "tool" {
			toolResults++
		}
	}
	sequence := []string{
		"get_host",
		"query_metrics",
		"query_alarms",
		"query_changes",
		"query_cmdb",
		"assess_fault",
	}
	if toolResults >= len(sequence) {
		return client.ToolChatMessage{
			Role:    "assistant",
			Content: "已完成原生 function calling 工具调查，请以确定性评分结果为准。",
		}, nil
	}
	name := sequence[toolResults]
	args := fmt.Sprintf(`{"host_id":%s}`, strconv.Quote(hostID))
	return client.ToolChatMessage{
		Role: "assistant",
		ToolCalls: []client.ToolCall{{
			ID:   fmt.Sprintf("call-%d", toolResults+1),
			Type: "function",
			Function: client.FunctionCall{
				Name:      name,
				Arguments: args,
			},
		}},
	}, nil
}

func extractHostID(message string) string {
	re := regexp.MustCompile(`host-\d{3}`)
	return re.FindString(message)
}
