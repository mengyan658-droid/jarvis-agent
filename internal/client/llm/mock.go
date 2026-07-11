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
	if since := parseSince(message); since != "" {
		params["since"] = since
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

func parseSince(message string) string {
	switch {
	case strings.Contains(message, "最近一小时") || strings.Contains(message, "近一小时"):
		return "1h"
	case strings.Contains(message, "最近一周") || strings.Contains(message, "近一周") || strings.Contains(message, "过去一周"):
		return "1w"
	case strings.Contains(message, "今天"):
		return "today"
	case strings.Contains(message, "昨天"):
		return "yesterday"
	}
	if since := parseCompactSince(message); since != "" {
		return since
	}
	return parseChineseSince(message)
}

func parseCompactSince(message string) string {
	matches := regexp.MustCompile(`(?i)(?:最近|近|过去|last\s*)?(\d+)\s*([mhdw])\b`).FindStringSubmatch(message)
	if len(matches) != 3 {
		return ""
	}
	amount, err := strconv.Atoi(matches[1])
	if err != nil || amount <= 0 {
		return ""
	}
	return strconv.Itoa(amount) + strings.ToLower(matches[2])
}

func parseChineseSince(message string) string {
	if !(strings.Contains(message, "最近") || strings.Contains(message, "近") || strings.Contains(message, "过去")) {
		return ""
	}
	matches := regexp.MustCompile(`([0-9一二两三四五六七八九十]+)\s*(分钟|小时|天|日|周|星期)`).FindStringSubmatch(message)
	if len(matches) != 3 {
		return ""
	}
	amount, ok := parseSmallChineseNumber(matches[1])
	if !ok || amount <= 0 {
		return ""
	}
	unit := "d"
	switch matches[2] {
	case "分钟":
		unit = "m"
	case "小时":
		unit = "h"
	case "周", "星期":
		unit = "w"
	}
	return strconv.Itoa(amount) + unit
}

func parseSmallChineseNumber(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	values := map[rune]int{
		'一': 1,
		'二': 2,
		'两': 2,
		'三': 3,
		'四': 4,
		'五': 5,
		'六': 6,
		'七': 7,
		'八': 8,
		'九': 9,
	}
	if s == "十" {
		return 10, true
	}
	runes := []rune(s)
	if len(runes) == 1 {
		n, ok := values[runes[0]]
		return n, ok
	}
	if len(runes) == 2 && runes[1] == '十' {
		n, ok := values[runes[0]]
		return n * 10, ok
	}
	if len(runes) == 2 && runes[0] == '十' {
		n, ok := values[runes[1]]
		return 10 + n, ok
	}
	if len(runes) == 3 && runes[1] == '十' {
		tens, ok1 := values[runes[0]]
		ones, ok2 := values[runes[2]]
		return tens*10 + ones, ok1 && ok2
	}
	return 0, false
}

func extractHostID(message string) string {
	re := regexp.MustCompile(`host-\d{3}`)
	return re.FindString(message)
}
