package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

const ModelErrorDailyReportWorkflowName = "model_error_daily_report"

type ModelErrorDailyReportWorkflow struct{}

type ModelErrorDailyReportResult struct {
	Query          ModelErrorDailyReportQuery `json:"query"`
	ReportMarkdown string                     `json:"report_markdown"`
	TotalCount     int                        `json:"total_count"`
	ByDeviceModel  []DimensionCount           `json:"by_device_model"`
	ByIDC          []DimensionCount           `json:"by_idc,omitempty"`
	ByTime         []TimeBucketCount          `json:"by_time"`
	Records        []domain.ErrorRequestCount `json:"records"`
}

type ModelErrorDailyReportQuery struct {
	TimeRange    domain.TimeRange       `json:"time_range"`
	DeviceModels []string               `json:"device_models"`
	IDCs         []string               `json:"idcs,omitempty"`
	ErrorCode    string                 `json:"error_code"`
	Aggregation  domain.TimeAggregation `json:"aggregation"`
}

type DimensionCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type TimeBucketCount struct {
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	Count       int       `json:"count"`
}

func (w ModelErrorDailyReportWorkflow) Name() string { return ModelErrorDailyReportWorkflowName }

func (w ModelErrorDailyReportWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	steps := []Step{}
	warnings := []string{}
	params := wfctx.Intent.Parameters
	query := ModelErrorDailyReportQuery{}
	timeRangeInput := reportTimeRangeInput(params)
	var records []domain.ErrorRequestCount
	report := ModelErrorDailyReportResult{}
	executor := NewGuidedStepExecutor(ctx, wfctx, &steps, &warnings)

	summary := ""
	if err := executor.Run(
		GuidedStep{Name: "plan_report_time_range", Run: func(exec *GuidedStepExecutor) error {
			planned, err := planReportTimeRange(exec)
			if err != nil {
				exec.Warn("llm time range planning failed; used normalized intent parameters")
				return nil
			}
			timeRangeInput = planned
			return nil
		}},
		GuidedStep{Name: "resolve_report_time_range", Run: func(exec *GuidedStepExecutor) error {
			out, err := exec.ExecuteTool(tool.ResolveTimeRangeToolName, timeRangeInput)
			if err != nil {
				return err
			}
			query.TimeRange = out.(domain.TimeRange)
			return nil
		}},
		GuidedStep{Name: "plan_error_count_query", Run: func(exec *GuidedStepExecutor) error {
			query = reportQueryFromParameters(params, query.TimeRange)
			planned, err := planErrorCountQuery(exec, query.TimeRange)
			if err != nil {
				exec.Warn("llm report query planning failed; used normalized intent parameters")
				return nil
			}
			mergeReportQueryPlan(&query, planned)
			return nil
		}},
		GuidedStep{Name: "validate_report_parameters", Run: func(exec *GuidedStepExecutor) error {
			return validateReportQuery(query)
		}},
		GuidedStep{Name: "query_error_request_counts", Run: func(exec *GuidedStepExecutor) error {
			out, err := exec.ExecuteTool(tool.QueryErrorRequestCountsToolName, tool.QueryErrorRequestCountsInput{
				TimeRange:    query.TimeRange,
				DeviceModels: query.DeviceModels,
				IDCs:         query.IDCs,
				ErrorCode:    query.ErrorCode,
				Aggregation:  query.Aggregation,
			})
			if err != nil {
				return err
			}
			records = out.([]domain.ErrorRequestCount)
			return nil
		}},
		GuidedStep{Name: "analyze_model_dimension", Run: func(exec *GuidedStepExecutor) error {
			report.Query = query
			report.Records = records
			report.TotalCount = sumErrorRequestCounts(records)
			report.ByDeviceModel = dimensionCounts(records, func(record domain.ErrorRequestCount) string {
				return record.DeviceModel
			})
			report.ByIDC = dimensionCounts(records, func(record domain.ErrorRequestCount) string {
				return record.IDC
			})
			return nil
		}},
		GuidedStep{Name: "analyze_time_dimension", Run: func(exec *GuidedStepExecutor) error {
			report.ByTime = timeBucketCounts(records)
			return nil
		}},
		GuidedStep{Name: "generate_daily_report_summary", Run: func(exec *GuidedStepExecutor) error {
			var err error
			summary, err = wfctx.LLM.GenerateModelErrorDailyReport(ctx, report)
			if err != nil {
				summary = fallbackModelErrorDailyReportMarkdown(report)
				exec.Warn("llm daily report failed; used fallback markdown report")
				return nil
			}
			report.ReportMarkdown = summary
			return nil
		}},
	); err != nil {
		return Result{}, err
	}
	if report.ReportMarkdown == "" {
		report.ReportMarkdown = summary
	}

	return Result{
		Intent:    wfctx.Intent.Name,
		Workflow:  w.Name(),
		Summary:   summary,
		Results:   report,
		Warnings:  warnings,
		Steps:     steps,
		ToolCalls: wfctx.ToolRecorder.Calls(),
	}, nil
}

func reportTimeRangeInput(params map[string]string) tool.ResolveTimeRangeInput {
	if params == nil {
		return tool.ResolveTimeRangeInput{Kind: "relative", Amount: 24, Unit: "hour"}
	}
	if strings.TrimSpace(params["start_text"]) != "" || strings.TrimSpace(params["end_text"]) != "" {
		return timeRangeInputFromParameters(params)
	}
	if since := strings.TrimSpace(params["since"]); since != "" {
		return timeRangeInputFromText(since)
	}
	return tool.ResolveTimeRangeInput{Kind: "relative", Amount: 24, Unit: "hour"}
}

func planReportTimeRange(exec *GuidedStepExecutor) (tool.ResolveTimeRangeInput, error) {
	call, err := exec.PlanToolCall(GuidedToolPlanRequest{
		SystemPrompt: "你是日报查询的时间参数规划器。你必须调用 resolve_time_range，只根据用户原文生成时间范围参数，不要生成时间戳。默认时间范围是最近 24 小时；最近一周使用 amount=1 unit=week。",
		Tool:         tool.ResolveTimeRangeFunctionTool(),
	})
	if err != nil {
		return tool.ResolveTimeRangeInput{}, err
	}
	input, err := DecodeGuidedToolArguments[tool.ResolveTimeRangeInput](call)
	if err != nil {
		return tool.ResolveTimeRangeInput{}, err
	}
	return normalizeReportTimeRangePlan(input), nil
}

func normalizeReportTimeRangePlan(input tool.ResolveTimeRangeInput) tool.ResolveTimeRangeInput {
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind == "" || input.Kind == "default" {
		input.Kind = "relative"
		input.Amount = 24
		input.Unit = "hour"
	}
	return input
}

func planErrorCountQuery(exec *GuidedStepExecutor, timeRange domain.TimeRange) (ModelErrorDailyReportQuery, error) {
	call, err := exec.PlanToolCall(GuidedToolPlanRequest{
		SystemPrompt: "你是日报查询参数规划器。你必须调用 query_error_request_counts。只从用户原文提取 device_models、idcs、error_code 和 aggregation，不要生成时间戳；时间范围已经由本地 resolve_time_range 确定。数组参数必须传字符串数组。默认 aggregation_value=1 aggregation_unit=h。",
		Messages: []client.ToolChatMessage{
			{Role: "assistant", Content: fmt.Sprintf("已解析时间范围：%s 至 %s。请继续规划 query_error_request_counts 的业务查询参数。", timeRange.Start.Format(time.RFC3339), timeRange.End.Format(time.RFC3339))},
		},
		Tool: tool.QueryErrorRequestCountsFunctionTool(),
	})
	if err != nil {
		return ModelErrorDailyReportQuery{}, err
	}
	return decodeReportQueryPlan(call.Function.Arguments, timeRange)
}

func reportQueryFromParameters(params map[string]string, timeRange domain.TimeRange) ModelErrorDailyReportQuery {
	return ModelErrorDailyReportQuery{
		TimeRange:    timeRange,
		DeviceModels: parameterStringList(params, "device_models", "device_model", "models", "model"),
		IDCs:         parameterStringList(params, "idcs", "idc"),
		ErrorCode:    firstNonEmpty(params, "error_code", "errorCode", "code"),
		Aggregation:  aggregationFromParameters(params),
	}
}

func mergeReportQueryPlan(query *ModelErrorDailyReportQuery, planned ModelErrorDailyReportQuery) {
	if query == nil {
		return
	}
	if len(planned.DeviceModels) > 0 {
		query.DeviceModels = planned.DeviceModels
	}
	if planned.IDCs != nil {
		query.IDCs = planned.IDCs
	}
	if planned.ErrorCode != "" {
		query.ErrorCode = planned.ErrorCode
	}
	if _, err := planned.Aggregation.Duration(); err == nil {
		query.Aggregation = planned.Aggregation
	}
}

func validateReportQuery(query ModelErrorDailyReportQuery) error {
	if len(query.DeviceModels) == 0 {
		return fmt.Errorf("device_models is required")
	}
	if query.ErrorCode == "" {
		return fmt.Errorf("error_code is required")
	}
	if _, err := query.Aggregation.Duration(); err != nil {
		return err
	}
	return nil
}

func decodeReportQueryPlan(arguments string, timeRange domain.TimeRange) (ModelErrorDailyReportQuery, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return ModelErrorDailyReportQuery{}, fmt.Errorf("decode query_error_request_counts arguments: %w", err)
	}
	query := ModelErrorDailyReportQuery{
		TimeRange:    timeRange,
		DeviceModels: anyStringList(raw["device_models"]),
		IDCs:         anyStringList(raw["idcs"]),
		ErrorCode:    strings.ToUpper(strings.TrimSpace(anyString(raw["error_code"]))),
		Aggregation: domain.TimeAggregation{
			Value: anyInt(raw["aggregation_value"], 1),
			Unit:  anyStringWithDefault(raw["aggregation_unit"], "h"),
		},
	}
	return query, nil
}

func anyStringList(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, anyString(item))
		}
		return compactStrings(out)
	case []string:
		return compactStrings(v)
	case string:
		return parseStringList(v)
	default:
		return nil
	}
}

func anyString(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func anyStringWithDefault(value any, fallback string) string {
	if s := anyString(value); s != "" {
		return s
	}
	return fallback
}

func anyInt(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func parameterStringList(params map[string]string, keys ...string) []string {
	for _, key := range keys {
		values := parseStringList(params[key])
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var values []string
		if err := json.Unmarshal([]byte(value), &values); err == nil {
			return compactStrings(values)
		}
	}
	replacer := strings.NewReplacer("，", ",", "、", ",", ";", ",", "；", ",", "\n", ",")
	parts := strings.Split(replacer.Replace(value), ",")
	return compactStrings(parts)
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(params map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			return strings.ToUpper(value)
		}
	}
	return ""
}

func aggregationFromParameters(params map[string]string) domain.TimeAggregation {
	value := intParameter(params, 1, "aggregation_value", "interval_value", "bucket_value")
	unit := strings.TrimSpace(firstRawNonEmpty(params, "aggregation_unit", "interval_unit", "bucket_unit"))
	if unit == "" {
		if amount, parsedUnit, ok := parseCompactAggregation(params["aggregation"]); ok {
			return domain.TimeAggregation{Value: amount, Unit: parsedUnit}
		}
		unit = "h"
	}
	return domain.TimeAggregation{Value: value, Unit: unit}
}

func intParameter(params map[string]string, fallback int, keys ...string) int {
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			parsed, err := strconv.Atoi(value)
			if err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func firstRawNonEmpty(params map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			return value
		}
	}
	return ""
}

func parseCompactAggregation(value string) (int, string, bool) {
	matches := compactAggregationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 3 {
		return 0, "", false
	}
	amount, err := strconv.Atoi(matches[1])
	if err != nil || amount <= 0 {
		return 0, "", false
	}
	return amount, matches[2], true
}

var compactAggregationPattern = regexp.MustCompile(`(?i)^(\d+)\s*([mhd])$`)

func sumErrorRequestCounts(records []domain.ErrorRequestCount) int {
	total := 0
	for _, record := range records {
		total += record.Count
	}
	return total
}

func dimensionCounts(records []domain.ErrorRequestCount, keyFn func(domain.ErrorRequestCount) string) []DimensionCount {
	counts := map[string]int{}
	for _, record := range records {
		key := keyFn(record)
		if key == "" {
			key = "unknown"
		}
		counts[key] += record.Count
	}
	out := make([]DimensionCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, DimensionCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func timeBucketCounts(records []domain.ErrorRequestCount) []TimeBucketCount {
	counts := map[int64]*TimeBucketCount{}
	for _, record := range records {
		key := record.BucketStart.UnixNano()
		bucket, ok := counts[key]
		if !ok {
			bucket = &TimeBucketCount{BucketStart: record.BucketStart, BucketEnd: record.BucketEnd}
			counts[key] = bucket
		}
		bucket.Count += record.Count
	}
	out := make([]TimeBucketCount, 0, len(counts))
	for _, bucket := range counts {
		out = append(out, *bucket)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BucketStart.Before(out[j].BucketStart)
	})
	return out
}

func fallbackModelErrorDailyReportMarkdown(report ModelErrorDailyReportResult) string {
	models := strings.Join(report.Query.DeviceModels, "、")
	idcs := strings.Join(report.Query.IDCs, "、")
	if idcs == "" {
		idcs = "全部"
	}
	start := report.Query.TimeRange.Start.Format("2006-01-02 15:04")
	end := report.Query.TimeRange.End.Format("2006-01-02 15:04")
	var b strings.Builder
	b.WriteString("# 机型错误码数量日报\n\n")
	b.WriteString("## 概览\n\n")
	b.WriteString(fmt.Sprintf("- 总错误数：%d\n", report.TotalCount))
	b.WriteString(fmt.Sprintf("- 错误码：%s\n", report.Query.ErrorCode))
	b.WriteString(fmt.Sprintf("- 机型范围：%s\n", models))
	b.WriteString(fmt.Sprintf("- 时间范围：%s 至 %s\n", start, end))
	if report.TotalCount == 0 {
		b.WriteString("\n## 查询条件\n\n")
		b.WriteString(fmt.Sprintf("- IDC：%s\n", idcs))
		b.WriteString(fmt.Sprintf("- 聚合粒度：%d%s\n", report.Query.Aggregation.Value, report.Query.Aggregation.Unit))
		b.WriteString("\n## 机型维度分析\n\n无匹配数据。\n")
		b.WriteString("\n## 时间趋势分析\n\n查询范围内没有匹配数据，无法形成时间趋势。\n")
		b.WriteString("\n## IDC 维度分析\n\n无匹配数据。\n")
		b.WriteString("\n## 明细数据\n\n无明细数据。\n")
		b.WriteString("\n## 结论\n\n查询范围内未发现匹配错误请求。\n")
		return b.String()
	}
	topModel := report.ByDeviceModel[0]
	peak := report.ByTime[0]
	low := report.ByTime[0]
	for _, bucket := range report.ByTime {
		if bucket.Count > peak.Count {
			peak = bucket
		}
		if bucket.Count < low.Count {
			low = bucket
		}
	}
	b.WriteString(fmt.Sprintf("- 主要机型：%s（%d 次）\n", topModel.Name, topModel.Count))
	b.WriteString(fmt.Sprintf("- 峰值时间段：%s 至 %s（%d 次）\n", peak.BucketStart.Format("2006-01-02 15:04"), peak.BucketEnd.Format("15:04"), peak.Count))
	b.WriteString("\n## 查询条件\n\n")
	b.WriteString(fmt.Sprintf("- IDC：%s\n", idcs))
	b.WriteString(fmt.Sprintf("- 聚合粒度：%d%s\n", report.Query.Aggregation.Value, report.Query.Aggregation.Unit))
	b.WriteString("\n## 机型维度分析\n\n")
	writeWorkflowDimensionTable(&b, report.ByDeviceModel)
	b.WriteString("\n## 时间趋势分析\n\n")
	b.WriteString(fmt.Sprintf("峰值出现在 %s 至 %s，共 %d 次；低谷出现在 %s 至 %s，共 %d 次。整体趋势请结合业务流量基线进一步判断。\n",
		peak.BucketStart.Format("2006-01-02 15:04"), peak.BucketEnd.Format("15:04"), peak.Count,
		low.BucketStart.Format("2006-01-02 15:04"), low.BucketEnd.Format("15:04"), low.Count))
	b.WriteString("\n## IDC 维度分析\n\n")
	writeWorkflowDimensionTable(&b, report.ByIDC)
	b.WriteString("\n## 明细数据\n\n")
	writeWorkflowRecordTable(&b, report.Records)
	b.WriteString("\n## 结论\n\n")
	b.WriteString("错误请求主要集中在高计数机型和峰值时间段，建议结合该时间段的发布、流量变化和服务端日志继续排查。\n")
	return b.String()
}

func writeWorkflowDimensionTable(b *strings.Builder, rows []DimensionCount) {
	if len(rows) == 0 {
		b.WriteString("无匹配数据。\n")
		return
	}
	b.WriteString("| 名称 | 错误数 |\n| --- | ---: |\n")
	for _, row := range rows {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", row.Name, row.Count))
	}
}

func writeWorkflowRecordTable(b *strings.Builder, rows []domain.ErrorRequestCount) {
	if len(rows) == 0 {
		b.WriteString("无明细数据。\n")
		return
	}
	b.WriteString("| 时间段 | 机型 | IDC | 错误码 | 错误数 |\n| --- | --- | --- | --- | ---: |\n")
	limit := len(rows)
	if limit > 20 {
		limit = 20
	}
	for _, row := range rows[:limit] {
		b.WriteString(fmt.Sprintf("| %s - %s | %s | %s | %s | %d |\n",
			row.BucketStart.Format("2006-01-02 15:04"),
			row.BucketEnd.Format("15:04"),
			row.DeviceModel,
			row.IDC,
			row.ErrorCode,
			row.Count))
	}
}
