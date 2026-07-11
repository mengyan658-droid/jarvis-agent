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
	Model       string                   `json:"model"`
	Messages    []client.ToolChatMessage `json:"messages"`
	Tools       []client.FunctionTool    `json:"tools,omitempty"`
	ToolChoice  string                   `json:"tool_choice,omitempty"`
	Stream      bool                     `json:"stream"`
	Temperature float64                  `json:"temperature"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message client.ToolChatMessage `json:"message"`
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
	content, err := c.chat(ctx, "parse_intent", []client.ToolChatMessage{
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
	return c.chat(ctx, "generate_fault_summary", []client.ToolChatMessage{
		{Role: "system", Content: "你是基础设施运维助手。基于输入的故障评分结果生成一句中文摘要，不要编造输入中不存在的信息。"},
		{Role: "user", Content: string(payload)},
	}, 0.2)
}

func (c *OpenAICompatibleClient) GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error) {
	payload, err := json.Marshal(assessment)
	if err != nil {
		return "", err
	}
	return c.chat(ctx, "generate_host_diagnosis", []client.ToolChatMessage{
		{Role: "system", Content: "你是基础设施运维助手。基于输入的单机故障评分、证据和原因生成简短中文诊断结论。评分结果必须以输入为准，不要重新打分。"},
		{Role: "user", Content: string(payload)},
	}, 0.2)
}

func (c *OpenAICompatibleClient) ChatWithTools(ctx context.Context, messages []client.ToolChatMessage, tools []client.FunctionTool) (client.ToolChatMessage, error) {
	return c.chatCompletion(ctx, "function_calling", messages, tools, "auto", 0)
}

func (c *OpenAICompatibleClient) chat(ctx context.Context, operation string, messages []client.ToolChatMessage, temperature float64) (string, error) {
	message, err := c.chatCompletion(ctx, operation, messages, nil, "", temperature)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(message.Content), nil
}

func (c *OpenAICompatibleClient) chatCompletion(ctx context.Context, operation string, messages []client.ToolChatMessage, tools []client.FunctionTool, toolChoice string, temperature float64) (client.ToolChatMessage, error) {
	if c.BaseURL == "" {
		return client.ToolChatMessage{}, errors.New("llm api base url is required")
	}
	if c.APIKey == "" {
		return client.ToolChatMessage{}, errors.New("llm api key is required")
	}
	if c.Model == "" {
		return client.ToolChatMessage{}, errors.New("llm model is required")
	}

	reqBody, err := json.Marshal(chatCompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  toolChoice,
		Stream:      false,
		Temperature: temperature,
	})
	if err != nil {
		return client.ToolChatMessage{}, err
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
			"tools", len(tools),
		)
	}
	if c.Logger != nil {
		c.Logger.Info("llm request body", "operation", operation, "body", string(reqBody))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return client.ToolChatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return client.ToolChatMessage{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return client.ToolChatMessage{}, err
	}
	if c.Logger != nil {
		c.Logger.Info("llm response body", "operation", operation, "status", resp.StatusCode, "body", string(body))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return client.ToolChatMessage{}, fmt.Errorf("decode llm response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return client.ToolChatMessage{}, fmt.Errorf("llm api error: status=%d message=%s", resp.StatusCode, decoded.Error.Message)
		}
		return client.ToolChatMessage{}, fmt.Errorf("llm api error: status=%d body=%s", resp.StatusCode, string(body))
	}
	if len(decoded.Choices) == 0 {
		return client.ToolChatMessage{}, errors.New("llm api returned no choices")
	}
	return decoded.Choices[0].Message, nil
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
{"name":"query_faulty_hosts|diagnose_host|tool_loop_investigate_host|unknown","parameters":{"region":"","environment":"","since":"","start_text":"","end_text":"","host_id":""}}

规则：
- “故障机”“异常机器” => query_faulty_hosts
- “诊断 host-001”“分析 host-001” => diagnose_host
- “排查 host-001”“根因 host-001” => tool_loop_investigate_host
- “华东” => region=east-china
- “华北” => region=north-china
- “华南” => region=south-china
- “生产” => environment=production
- “预发”“测试” => environment=staging
- “最近N分钟”“近N分钟” => since=Nm，例如最近30分钟 => since=30m
- “最近N小时”“近N小时” => since=Nh，例如最近5小时 => since=5h
- “最近N天”“近N天” => since=Nd，例如最近2天 => since=2d
- “最近N周”“近N周” => since=Nw，例如近一周 => since=1w
- “今天” => since=today
- “昨天” => since=yesterday
- 用户给出明确起止时间，例如“7月1号到7月5号”“2026-07-01 到 2026-07-05” => start_text=原始开始时间文本，end_text=原始结束时间文本，since 留空
- 提取 host-001 这类 Host ID 到 host_id
- 不要输出具体时间戳，时间戳由 resolve_time_range 工具在本地计算
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
