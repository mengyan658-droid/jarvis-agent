package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

const assessFaultFunctionName = "assess_fault"

type FunctionCallingTrace struct {
	Step         int    `json:"step"`
	ToolCallID   string `json:"tool_call_id,omitempty"`
	Function     string `json:"function"`
	Arguments    string `json:"arguments,omitempty"`
	CanonicalKey string `json:"canonical_key,omitempty"`
	Status       string `json:"status,omitempty"`
	Observation  string `json:"observation,omitempty"`
	Error        string `json:"error,omitempty"`
}

const ToolLoopInvestigateHostWorkflowName = "tool_loop_investigate_host"

type ToolLoopInvestigationResult struct {
	Trace      []FunctionCallingTrace `json:"function_call_trace"`
	Final      string                 `json:"final"`
	Assessment domain.FaultAssessment `json:"assessment"`
}

type ToolLoopInvestigateHostWorkflow struct{}

type ToolObservation struct {
	Status               string   `json:"status"`
	Function             string   `json:"function"`
	CanonicalKey         string   `json:"canonical_key"`
	Summary              string   `json:"summary"`
	NextAllowedFunctions []string `json:"next_allowed_functions,omitempty"`
}

type toolLoopState struct {
	executed       map[string]ToolObservation
	completed      map[string]bool
	evidence       domain.FaultEvidence
	assessment     domain.FaultAssessment
	assessmentDone bool
}

type normalizedFunctionCall struct {
	Name         string
	HostID       string
	Arguments    string
	CanonicalKey string
}

func (w ToolLoopInvestigateHostWorkflow) Name() string { return ToolLoopInvestigateHostWorkflowName }

func (w ToolLoopInvestigateHostWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	functionLLM, ok := wfctx.LLM.(client.FunctionCallingClient)
	if !ok {
		return Result{}, errors.New("llm client does not support function calling")
	}

	steps := []Step{}
	warnings := []string{}
	trace := []FunctionCallingTrace{}
	var hostID string
	state := newToolLoopState()
	final := ""

	if err := runStep(&steps, "function_call_extract_host_id", func() error {
		var err error
		hostID, err = requireHostID(wfctx.Intent.Parameters)
		return err
	}); err != nil {
		return Result{}, err
	}

	messages := []client.ToolChatMessage{
		{Role: "system", Content: functionCallingSystemPrompt()},
		{Role: "user", Content: fmt.Sprintf("请用可用函数排查 %s 的根因。必须先调用工具收集证据，最后给出简短中文结论。", hostID)},
	}
	maxCalls := wfctx.MaxSteps
	if maxCalls <= 0 {
		maxCalls = 10
	}

	for calls := 0; calls < maxCalls; calls++ {
		tools := availableInvestigationTools(state)
		if len(tools) == 0 {
			break
		}
		var assistant client.ToolChatMessage
		if err := runStep(&steps, fmt.Sprintf("function_call_llm_round:%d", calls+1), func() error {
			var err error
			assistant, err = functionLLM.ChatWithTools(ctx, messages, tools)
			return err
		}); err != nil {
			return Result{}, err
		}

		messages = append(messages, assistant)
		if len(assistant.ToolCalls) == 0 {
			final = assistant.Content
			break
		}

		for _, call := range assistant.ToolCalls {
			normalized, observation, err := handleFunctionCall(ctx, wfctx, call, hostID, state)
			record := FunctionCallingTrace{
				Step:         len(trace) + 1,
				ToolCallID:   call.ID,
				Function:     normalized.Name,
				Arguments:    normalized.Arguments,
				CanonicalKey: normalized.CanonicalKey,
			}
			if err != nil {
				record.Error = err.Error()
				trace = append(trace, record)
				return Result{}, err
			}
			record.Status = observation.Status
			record.Observation = observation.Summary
			trace = append(trace, record)
			messages = append(messages, client.ToolChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    client.MarshalToolResult(observation),
			})
		}
		if state.assessmentDone {
			var err error
			final, err = wfctx.LLM.GenerateHostDiagnosis(ctx, state.assessment)
			if err != nil {
				final = "工具调查已完成，诊断摘要生成失败，已返回确定性评分结果。"
				warnings = append(warnings, "llm diagnosis failed after assess_fault; used fallback summary")
			}
			break
		}
	}

	if final == "" {
		warnings = append(warnings, "function calling reached max rounds before final answer")
		final = "函数调用已完成，但模型未在限制轮次内生成最终结论。"
	}
	if state.assessment.HostID == "" {
		state.assessment = wfctx.Analyzer.Analyze(state.evidence)
	}

	return Result{
		Intent:   wfctx.Intent.Name,
		Workflow: w.Name(),
		Summary:  final,
		Results: ToolLoopInvestigationResult{
			Trace:      trace,
			Final:      final,
			Assessment: state.assessment,
		},
		Warnings:  warnings,
		Steps:     steps,
		ToolCalls: wfctx.ToolRecorder.Calls(),
	}, nil
}

func newToolLoopState() *toolLoopState {
	return &toolLoopState{
		executed:  map[string]ToolObservation{},
		completed: map[string]bool{},
	}
}

func handleFunctionCall(ctx context.Context, wfctx Context, call client.ToolCall, fallbackHostID string, state *toolLoopState) (normalizedFunctionCall, ToolObservation, error) {
	normalized, err := normalizeFunctionCall(call, fallbackHostID)
	if err != nil {
		return normalizedFunctionCall{}, ToolObservation{}, err
	}
	if previous, ok := state.executed[normalized.CanonicalKey]; ok {
		observation := previous
		observation.Status = "skipped_duplicate"
		observation.Summary = fmt.Sprintf("%s was already executed; reused observation %s", normalized.Name, normalized.CanonicalKey)
		observation.NextAllowedFunctions = nextAllowedFunctions(state)
		return normalized, observation, nil
	}
	if normalized.Name == assessFaultFunctionName && !evidenceReady(state) {
		observation := ToolObservation{
			Status:               "blocked_prerequisite",
			Function:             normalized.Name,
			CanonicalKey:         normalized.CanonicalKey,
			Summary:              "cannot assess fault before required evidence tools complete: " + strings.Join(missingEvidenceFunctions(state), ","),
			NextAllowedFunctions: nextAllowedFunctions(state),
		}
		return normalized, observation, nil
	}

	result, err := executeNormalizedFunctionCall(ctx, wfctx, normalized, state)
	if err != nil {
		return normalized, ToolObservation{}, err
	}
	observation := ToolObservation{
		Status:               "complete",
		Function:             normalized.Name,
		CanonicalKey:         normalized.CanonicalKey,
		Summary:              summarizeToolResult(normalized.Name, result),
		NextAllowedFunctions: nextAllowedFunctions(state),
	}
	state.executed[normalized.CanonicalKey] = observation
	return normalized, observation, nil
}

func executeNormalizedFunctionCall(ctx context.Context, wfctx Context, normalized normalizedFunctionCall, state *toolLoopState) (any, error) {
	hostID := normalized.HostID
	switch normalized.Name {
	case tool.GetHostToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.GetHostToolName, tool.GetHostInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		state.evidence.Host = out.(domain.Host)
		state.completed[tool.GetHostToolName] = true
		return state.evidence.Host, nil
	case tool.QueryMetricsToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryMetricsToolName, tool.QueryMetricsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		state.evidence.Metrics = out.(domain.HostMetrics)
		state.completed[tool.QueryMetricsToolName] = true
		return state.evidence.Metrics, nil
	case tool.QueryAlarmsToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryAlarmsToolName, tool.QueryAlarmsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		state.evidence.Alarms = out.([]domain.Alarm)
		state.completed[tool.QueryAlarmsToolName] = true
		return state.evidence.Alarms, nil
	case tool.QueryChangesToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryChangesToolName, tool.QueryChangesInput{HostID: hostID, Since: time.Now().Add(-1 * time.Hour)}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		state.evidence.RecentChanges = out.([]domain.ChangeRecord)
		state.completed[tool.QueryChangesToolName] = true
		return state.evidence.RecentChanges, nil
	case tool.QueryCMDBToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryCMDBToolName, tool.QueryCMDBInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		state.evidence.CMDB = out.(map[string]string)
		state.completed[tool.QueryCMDBToolName] = true
		return state.evidence.CMDB, nil
	case assessFaultFunctionName:
		state.assessment = wfctx.Analyzer.Analyze(state.evidence)
		state.assessmentDone = true
		state.completed[assessFaultFunctionName] = true
		return state.assessment, nil
	default:
		return nil, fmt.Errorf("unsupported function call %q", normalized.Name)
	}
}

func normalizeFunctionCall(call client.ToolCall, fallbackHostID string) (normalizedFunctionCall, error) {
	name := normalizeFunctionName(call.Function.Name)
	hostID, err := functionHostID(call.Function.Arguments, fallbackHostID)
	if err != nil {
		return normalizedFunctionCall{}, err
	}
	args := map[string]string{"host_id": hostID}
	argumentBytes, err := json.Marshal(args)
	if err != nil {
		return normalizedFunctionCall{}, err
	}
	normalized := normalizedFunctionCall{
		Name:      name,
		HostID:    hostID,
		Arguments: string(argumentBytes),
	}
	normalized.CanonicalKey = canonicalFunctionKey(name, hostID)
	if normalized.CanonicalKey == "" {
		return normalized, fmt.Errorf("unsupported function call %q", call.Function.Name)
	}
	return normalized, nil
}

func normalizeFunctionName(name string) string {
	switch strings.TrimSpace(name) {
	case "get_host", "query_host", "getHost":
		return tool.GetHostToolName
	case "query_metrics", "query_host_metrics", "get_metrics":
		return tool.QueryMetricsToolName
	case "query_alarms", "query_active_alarms", "get_alarms":
		return tool.QueryAlarmsToolName
	case "query_changes", "query_recent_changes", "get_changes":
		return tool.QueryChangesToolName
	case "query_cmdb", "query_host_metadata", "get_cmdb":
		return tool.QueryCMDBToolName
	case "assess_fault", "fault_analyzer", "analyze_fault":
		return assessFaultFunctionName
	default:
		return strings.TrimSpace(name)
	}
}

func canonicalFunctionKey(name, hostID string) string {
	switch name {
	case tool.GetHostToolName:
		return "get_host:" + hostID
	case tool.QueryMetricsToolName:
		return "query_metrics:" + hostID + ":last_1h"
	case tool.QueryAlarmsToolName:
		return "query_alarms:" + hostID
	case tool.QueryChangesToolName:
		return "query_changes:" + hostID + ":last_1h"
	case tool.QueryCMDBToolName:
		return "query_cmdb:" + hostID
	case assessFaultFunctionName:
		return "assess_fault:" + hostID + ":evidence_v1"
	default:
		return ""
	}
}

func functionHostID(arguments, fallback string) (string, error) {
	if arguments == "" {
		return normalizeHostID(fallback), nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return "", fmt.Errorf("decode function arguments: %w", err)
	}
	for _, key := range []string{"host_id", "hostID", "host", "id"} {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return normalizeHostID(value), nil
		}
	}
	return normalizeHostID(fallback), nil
}

func normalizeHostID(hostID string) string {
	return strings.ToLower(strings.TrimSpace(hostID))
}

func availableInvestigationTools(state *toolLoopState) []client.FunctionTool {
	if state.assessmentDone {
		return nil
	}
	if evidenceReady(state) {
		if state.completed[assessFaultFunctionName] {
			return nil
		}
		return []client.FunctionTool{functionTool(assessFaultFunctionName, "基于已收集证据执行确定性故障评分", hostIDSchema())}
	}
	missing := missingEvidenceFunctions(state)
	tools := make([]client.FunctionTool, 0, len(missing))
	for _, name := range missing {
		tools = append(tools, functionTool(name, evidenceFunctionDescription(name), hostIDSchema()))
	}
	return tools
}

func evidenceReady(state *toolLoopState) bool {
	return len(missingEvidenceFunctions(state)) == 0
}

func missingEvidenceFunctions(state *toolLoopState) []string {
	required := []string{
		tool.GetHostToolName,
		tool.QueryMetricsToolName,
		tool.QueryAlarmsToolName,
		tool.QueryChangesToolName,
		tool.QueryCMDBToolName,
	}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if !state.completed[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func nextAllowedFunctions(state *toolLoopState) []string {
	tools := availableInvestigationTools(state)
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Function.Name)
	}
	sort.Strings(names)
	return names
}

func evidenceFunctionDescription(name string) string {
	switch name {
	case tool.GetHostToolName:
		return "查询单台主机基础信息、可达性和健康检查状态"
	case tool.QueryMetricsToolName:
		return "查询单台主机 CPU、内存和持续高负载指标"
	case tool.QueryAlarmsToolName:
		return "查询单台主机当前活跃告警"
	case tool.QueryChangesToolName:
		return "查询单台主机最近一小时变更"
	case tool.QueryCMDBToolName:
		return "查询单台主机 CMDB 元数据"
	default:
		return "查询主机相关信息"
	}
}

func summarizeToolResult(name string, result any) string {
	switch v := result.(type) {
	case domain.Host:
		return fmt.Sprintf("host=%s reachable=%t health_check_passed=%t", v.ID, v.Reachable, v.HealthCheckPassed)
	case domain.HostMetrics:
		return fmt.Sprintf("cpu=%.0f memory=%.0f high_cpu_minutes=%d", v.CPUUsagePercent, v.MemoryUsagePercent, v.HighCPUDurationMinutes)
	case []domain.Alarm:
		return fmt.Sprintf("active_alarms=%d", len(v))
	case []domain.ChangeRecord:
		return fmt.Sprintf("recent_changes=%d", len(v))
	case map[string]string:
		return fmt.Sprintf("owner=%s service=%s", v["owner"], v["service"])
	case domain.FaultAssessment:
		return fmt.Sprintf("score=%d level=%s is_faulty=%t", v.Score, v.Level, v.IsFaulty)
	default:
		return fmt.Sprintf("%s returned", name)
	}
}

func hostIDSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host_id": map[string]any{"type": "string", "description": "Host ID, for example host-001"},
		},
		"required":             []string{"host_id"},
		"additionalProperties": false,
	}
}

func functionTool(name, description string, parameters map[string]any) client.FunctionTool {
	return client.FunctionTool{
		Type: "function",
		Function: client.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

func functionCallingSystemPrompt() string {
	return `你是基础设施运维 Agent。你必须使用原生 function calling 调用工具，不能编造工具结果。
每一轮只会给你当前阶段允许调用的工具。不要重复调用已经完成的工具。
排查单台主机时，先完成 get_host、query_metrics、query_alarms、query_changes、query_cmdb，然后调用 assess_fault 获取确定性评分。
最终回答要简短，说明故障等级、关键证据和可能根因。评分必须以 assess_fault 返回为准。`
}
