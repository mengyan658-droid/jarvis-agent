package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	return parseMockIntent(message), nil
}

func parseMockIntent(message string) client.Intent {
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
	if strings.Contains(message, "日报") || (strings.Contains(message, "错误码") && strings.Contains(message, "机型")) {
		name = "model_error_daily_report"
		if models := extractDeviceModels(message); len(models) > 0 {
			params["device_models"] = strings.Join(models, ",")
		}
		if idcs := extractIDCs(message); len(idcs) > 0 {
			params["idcs"] = strings.Join(idcs, ",")
		}
		if code := extractErrorCode(message); code != "" {
			params["error_code"] = code
		}
		params["aggregation_value"] = "1"
		params["aggregation_unit"] = "h"
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
	return client.Intent{Name: name, Parameters: params}
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

func (c *MockClient) GenerateModelErrorDailyReport(ctx context.Context, facts any) (string, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return "", err
	}
	payload, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	var report modelErrorDailyReportFacts
	if err := json.Unmarshal(payload, &report); err != nil {
		return "", err
	}
	return formatMockModelErrorDailyReport(report), nil
}

func (c *MockClient) ChatWithTools(ctx context.Context, messages []client.ToolChatMessage, tools []client.FunctionTool) (client.ToolChatMessage, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return client.ToolChatMessage{}, err
	}
	if isSelectSkillRequest(tools) {
		return mockSelectSkill(messages), nil
	}
	if hasFunctionTool(tools, "resolve_time_range") {
		return mockResolveTimeRange(messages), nil
	}
	if hasFunctionTool(tools, "query_error_request_counts") {
		return mockQueryErrorRequestCounts(messages), nil
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

type modelErrorDailyReportFacts struct {
	Query struct {
		TimeRange struct {
			Start time.Time `json:"start_time"`
			End   time.Time `json:"end_time"`
		} `json:"time_range"`
		DeviceModels []string               `json:"device_models"`
		IDCs         []string               `json:"idcs"`
		ErrorCode    string                 `json:"error_code"`
		Aggregation  domain.TimeAggregation `json:"aggregation"`
	} `json:"query"`
	TotalCount    int `json:"total_count"`
	ByDeviceModel []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"by_device_model"`
	ByIDC []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"by_idc"`
	ByTime []struct {
		BucketStart time.Time `json:"bucket_start"`
		BucketEnd   time.Time `json:"bucket_end"`
		Count       int       `json:"count"`
	} `json:"by_time"`
}

func formatMockModelErrorDailyReport(report modelErrorDailyReportFacts) string {
	peak, low := timeBucketExtremes(report.ByTime)
	models := strings.Join(report.Query.DeviceModels, "、")
	if models == "" {
		models = "-"
	}
	idcs := strings.Join(report.Query.IDCs, "、")
	if idcs == "" {
		idcs = "全部"
	}
	var b strings.Builder
	b.WriteString("# 机型错误码数量日报\n\n")
	b.WriteString("## 概览\n\n")
	b.WriteString(fmt.Sprintf("- 总错误数：%d\n", report.TotalCount))
	b.WriteString(fmt.Sprintf("- 错误码：%s\n", report.Query.ErrorCode))
	if len(report.ByDeviceModel) > 0 {
		b.WriteString(fmt.Sprintf("- 主要机型：%s（%d 次）\n", report.ByDeviceModel[0].Name, report.ByDeviceModel[0].Count))
	}
	if peak.Count > 0 {
		b.WriteString(fmt.Sprintf("- 峰值时间段：%s - %s（%d 次）\n", peak.BucketStart.Format("2006-01-02 15:04"), peak.BucketEnd.Format("15:04"), peak.Count))
	}
	b.WriteString("\n## 查询条件\n\n")
	b.WriteString(fmt.Sprintf("- 时间范围：%s 至 %s\n", report.Query.TimeRange.Start.Format("2006-01-02 15:04"), report.Query.TimeRange.End.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf("- 机型：%s\n", models))
	b.WriteString(fmt.Sprintf("- IDC：%s\n", idcs))
	b.WriteString(fmt.Sprintf("- 聚合粒度：%d%s\n", report.Query.Aggregation.Value, report.Query.Aggregation.Unit))
	b.WriteString("\n## 机型维度分析\n\n")
	writeDimensionMarkdown(&b, report.ByDeviceModel)
	b.WriteString("\n## 时间趋势分析\n\n")
	if len(report.ByTime) == 0 {
		b.WriteString("查询范围内没有匹配数据，无法形成时间趋势。\n")
	} else {
		b.WriteString(fmt.Sprintf("峰值出现在 %s 至 %s，共 %d 次；低谷出现在 %s 至 %s，共 %d 次。\n",
			peak.BucketStart.Format("2006-01-02 15:04"), peak.BucketEnd.Format("15:04"), peak.Count,
			low.BucketStart.Format("2006-01-02 15:04"), low.BucketEnd.Format("15:04"), low.Count))
	}
	b.WriteString("\n## IDC 维度分析\n\n")
	writeDimensionMarkdown(&b, report.ByIDC)
	b.WriteString("\n## 明细数据\n\n")
	writeTimeMarkdown(&b, report.ByTime)
	b.WriteString("\n## 结论\n\n")
	if report.TotalCount == 0 {
		b.WriteString("查询范围内未发现匹配错误请求。\n")
	} else {
		b.WriteString("错误请求主要集中在高计数机型和峰值时间段，建议结合该时间段的发布、流量和服务端日志继续排查。\n")
	}
	return b.String()
}

func timeBucketExtremes(buckets []struct {
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	Count       int       `json:"count"`
}) (peak, low struct {
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	Count       int       `json:"count"`
}) {
	if len(buckets) == 0 {
		return peak, low
	}
	peak = buckets[0]
	low = buckets[0]
	for _, bucket := range buckets[1:] {
		if bucket.Count > peak.Count {
			peak = bucket
		}
		if bucket.Count < low.Count {
			low = bucket
		}
	}
	return peak, low
}

func writeDimensionMarkdown(b *strings.Builder, rows []struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}) {
	if len(rows) == 0 {
		b.WriteString("无匹配数据。\n")
		return
	}
	b.WriteString("| 名称 | 错误数 |\n| --- | ---: |\n")
	for _, row := range rows {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", row.Name, row.Count))
	}
}

func writeTimeMarkdown(b *strings.Builder, rows []struct {
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	Count       int       `json:"count"`
}) {
	if len(rows) == 0 {
		b.WriteString("无明细数据。\n")
		return
	}
	b.WriteString("| 时间段 | 错误数 |\n| --- | ---: |\n")
	limit := len(rows)
	if limit > 20 {
		limit = 20
	}
	for _, row := range rows[:limit] {
		b.WriteString(fmt.Sprintf("| %s - %s | %d |\n", row.BucketStart.Format("2006-01-02 15:04"), row.BucketEnd.Format("15:04"), row.Count))
	}
}

func isSelectSkillRequest(tools []client.FunctionTool) bool {
	return hasFunctionTool(tools, "select_skill")
}

func hasFunctionTool(tools []client.FunctionTool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func mockResolveTimeRange(messages []client.ToolChatMessage) client.ToolChatMessage {
	userMessage := latestUserMessage(messages)
	payload := map[string]any{"kind": "relative", "amount": 24, "unit": "hour"}
	if since := parseSince(userMessage); since != "" {
		if amount, unit, ok := parseCompactLookback(since); ok {
			payload["amount"] = amount
			payload["unit"] = unit
		}
		if since == "today" {
			payload = map[string]any{"kind": "today"}
		}
		if since == "yesterday" {
			payload = map[string]any{"kind": "yesterday"}
		}
	}
	data, _ := json.Marshal(payload)
	return client.ToolChatMessage{
		Role: "assistant",
		ToolCalls: []client.ToolCall{{
			ID:   "call-resolve-time-range",
			Type: "function",
			Function: client.FunctionCall{
				Name:      "resolve_time_range",
				Arguments: string(data),
			},
		}},
	}
}

func mockQueryErrorRequestCounts(messages []client.ToolChatMessage) client.ToolChatMessage {
	userMessage := latestUserMessage(messages)
	payload := map[string]any{
		"device_models":     extractDeviceModels(userMessage),
		"idcs":              extractIDCs(userMessage),
		"error_code":        extractErrorCode(userMessage),
		"aggregation_value": 1,
		"aggregation_unit":  "h",
	}
	data, _ := json.Marshal(payload)
	return client.ToolChatMessage{
		Role: "assistant",
		ToolCalls: []client.ToolCall{{
			ID:   "call-query-error-request-counts",
			Type: "function",
			Function: client.FunctionCall{
				Name:      "query_error_request_counts",
				Arguments: string(data),
			},
		}},
	}
}

func latestUserMessage(messages []client.ToolChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func parseCompactLookback(since string) (int, string, bool) {
	matches := regexp.MustCompile(`(?i)^(\d+)([mhdw])$`).FindStringSubmatch(strings.TrimSpace(since))
	if len(matches) != 3 {
		return 0, "", false
	}
	amount, err := strconv.Atoi(matches[1])
	if err != nil || amount <= 0 {
		return 0, "", false
	}
	unit := map[string]string{"m": "minute", "h": "hour", "d": "day", "w": "week"}[strings.ToLower(matches[2])]
	return amount, unit, true
}

func mockSelectSkill(messages []client.ToolChatMessage) client.ToolChatMessage {
	userMessage := ""
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessage = msg.Content
		}
	}
	intent := parseMockIntent(userMessage)
	if intent.Name == "unknown" || intent.Name == "" {
		return client.ToolChatMessage{Role: "assistant", Content: "unknown"}
	}
	payload := map[string]any{
		"skill":      intent.Name,
		"parameters": intent.Parameters,
		"confidence": 0.9,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"skill":"unknown","parameters":{},"confidence":0}`)
	}
	return client.ToolChatMessage{
		Role: "assistant",
		ToolCalls: []client.ToolCall{{
			ID:   "call-select-skill",
			Type: "function",
			Function: client.FunctionCall{
				Name:      "select_skill",
				Arguments: string(data),
			},
		}},
	}
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

func extractDeviceModels(message string) []string {
	matches := regexp.MustCompile(`(?i)(iphone[-\s]?\d+|xiaomi[-\s]?\d+|huawei[-\s]?[a-z0-9]+|oppo[-\s]?[a-z0-9]+|vivo[-\s]?[a-z0-9]+)`).FindAllString(message, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		model := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(match), " ", "-"))
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	return out
}

func extractIDCs(message string) []string {
	matches := regexp.MustCompile(`(?i)(shanghai-a|beijing-a|shenzhen-a|上海a|北京a|深圳a)`).FindAllString(message, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		idc := strings.ToLower(strings.TrimSpace(match))
		switch idc {
		case "上海a":
			idc = "shanghai-a"
		case "北京a":
			idc = "beijing-a"
		case "深圳a":
			idc = "shenzhen-a"
		}
		if idc == "" || seen[idc] {
			continue
		}
		seen[idc] = true
		out = append(out, idc)
	}
	return out
}

func extractErrorCode(message string) string {
	matches := regexp.MustCompile(`(?i)(?:错误码|error\s*code|code)\s*(?:是|为|=|:|：)?\s*([A-Z][A-Z0-9_-]+)|\b(E[A-Z0-9_-]+)\b`).FindStringSubmatch(message)
	for _, match := range matches[1:] {
		if strings.TrimSpace(match) != "" {
			return strings.ToUpper(strings.TrimSpace(match))
		}
	}
	return ""
}
