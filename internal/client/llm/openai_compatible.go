package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
)

type OpenAICompatibleClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewOpenAICompatibleClient(baseURL, apiKey, model string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *OpenAICompatibleClient) WithLogger(logger *slog.Logger) *OpenAICompatibleClient {
	c.Logger = logger
	return c
}

func (c *OpenAICompatibleClient) ParseIntent(ctx context.Context, message string) (client.Intent, error) {
	content, err := c.chat(ctx, "parse_intent", []chatMessage{
		{Role: "system", Content: intentSystemPrompt()},
		{Role: "user", Content: message},
	}, 0)
	if err != nil {
		return client.Intent{}, err
	}
	var intent client.Intent
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &intent); err != nil {
		return client.Intent{}, fmt.Errorf("parse intent json: %w", err)
	}
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	return intent, nil
}

func (c *OpenAICompatibleClient) GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error) {
	payload, err := json.Marshal(assessments)
	if err != nil {
		return "", err
	}
	return c.chat(ctx, "generate_fault_summary", []chatMessage{
		{Role: "system", Content: "你是基础设施运维助手。基于输入的故障评分结果生成一句中文摘要，不要编造输入中不存在的信息。"},
		{Role: "user", Content: string(payload)},
	}, 0.2)
}

func (c *OpenAICompatibleClient) GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error) {
	payload, err := json.Marshal(assessment)
	if err != nil {
		return "", err
	}
	return c.chat(ctx, "generate_host_diagnosis", []chatMessage{
		{Role: "system", Content: "你是基础设施运维助手。基于输入的单机故障评分、证据和原因生成简短中文诊断结论。评分结果必须以输入为准，不要重新打分。"},
		{Role: "user", Content: string(payload)},
	}, 0.2)
}

func (c *OpenAICompatibleClient) chat(ctx context.Context, operation string, messages []chatMessage, temperature float64) (string, error) {
	if c.BaseURL == "" {
		return "", errors.New("llm api base url is required")
	}
	if c.APIKey == "" {
		return "", errors.New("llm api key is required")
	}
	if c.Model == "" {
		return "", errors.New("llm model is required")
	}

	reqBody, err := json.Marshal(chatCompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		Stream:      false,
		Temperature: temperature,
	})
	if err != nil {
		return "", err
	}

	url := c.chatCompletionsURL()
	if c.Logger != nil {
		c.Logger.Info("calling llm model",
			"operation", operation,
			"model", c.Model,
			"base_url", c.BaseURL,
			"url", url,
			"temperature", temperature,
			"messages", len(messages),
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return "", fmt.Errorf("llm api error: status=%d message=%s", resp.StatusCode, decoded.Error.Message)
		}
		return "", fmt.Errorf("llm api error: status=%d body=%s", resp.StatusCode, string(body))
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("llm api returned no choices")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func (c *OpenAICompatibleClient) chatCompletionsURL() string {
	if strings.HasSuffix(c.BaseURL, "/chat/completions") {
		return c.BaseURL
	}
	return c.BaseURL + "/chat/completions"
}

func intentSystemPrompt() string {
	return `你是 Jarvis Agent 的意图解析器，只能输出 JSON，不要输出 Markdown。
输出格式：
{"name":"query_faulty_hosts|diagnose_host|react_investigate_host|unknown","parameters":{"region":"","environment":"","since":"","host_id":""}}

规则：
- “故障机”“异常机器” => query_faulty_hosts
- “诊断 host-001”“分析 host-001” => diagnose_host
- “排查 host-001”“根因 host-001” => react_investigate_host
- “华东” => region=east-china
- “华北” => region=north-china
- “华南” => region=south-china
- “生产” => environment=production
- “预发”“测试” => environment=staging
- “最近一小时” => since=1h
- 提取 host-001 这类 Host ID 到 host_id
- 无法识别时 name=unknown`
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end >= start {
		return content[start : end+1]
	}
	return content
}
