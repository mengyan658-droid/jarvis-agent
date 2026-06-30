package llm

import (
	"context"
	"fmt"
	"regexp"
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
			name = "react_investigate_host"
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

func extractHostID(message string) string {
	re := regexp.MustCompile(`host-\d{3}`)
	return re.FindString(message)
}
