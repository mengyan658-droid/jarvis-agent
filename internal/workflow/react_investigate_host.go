package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

const assessFaultFunctionName = "assess_fault"

type FunctionCallingTrace struct {
	Step        int    `json:"step"`
	ToolCallID  string `json:"tool_call_id,omitempty"`
	Function    string `json:"function"`
	Arguments   string `json:"arguments,omitempty"`
	Observation string `json:"observation,omitempty"`
	Error       string `json:"error,omitempty"`
}

type ReactInvestigationResult struct {
	Trace      []FunctionCallingTrace `json:"function_call_trace"`
	Final      string                 `json:"final"`
	Assessment domain.FaultAssessment `json:"assessment"`
}

type ReactInvestigateHostWorkflow struct{}

func (w ReactInvestigateHostWorkflow) Name() string { return "react_investigate_host" }

func (w ReactInvestigateHostWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	functionLLM, ok := wfctx.LLM.(client.FunctionCallingClient)
	if !ok {
		return Result{}, errors.New("llm client does not support function calling")
	}

	steps := []Step{}
	warnings := []string{}
	trace := []FunctionCallingTrace{}
	var hostID string
	var evidence domain.FaultEvidence
	var assessment domain.FaultAssessment
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
	tools := investigationFunctionTools()
	maxCalls := wfctx.MaxSteps
	if maxCalls <= 0 {
		maxCalls = 10
	}

	for calls := 0; calls < maxCalls; calls++ {
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
			toolResult, err := executeFunctionCall(ctx, wfctx, call, hostID, &evidence, &assessment)
			record := FunctionCallingTrace{
				Step:       len(trace) + 1,
				ToolCallID: call.ID,
				Function:   call.Function.Name,
				Arguments:  call.Function.Arguments,
			}
			if err != nil {
				record.Error = err.Error()
				trace = append(trace, record)
				return Result{}, err
			}
			record.Observation = summarizeToolResult(call.Function.Name, toolResult)
			trace = append(trace, record)
			messages = append(messages, client.ToolChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    client.MarshalToolResult(toolResult),
			})
		}
	}

	if final == "" {
		warnings = append(warnings, "function calling reached max rounds before final answer")
		final = "函数调用已完成，但模型未在限制轮次内生成最终结论。"
	}
	if assessment.HostID == "" {
		assessment = wfctx.Analyzer.Analyze(evidence)
	}

	return Result{
		Intent:   wfctx.Intent.Name,
		Workflow: w.Name(),
		Summary:  final,
		Results: ReactInvestigationResult{
			Trace:      trace,
			Final:      final,
			Assessment: assessment,
		},
		Warnings:  warnings,
		Steps:     steps,
		ToolCalls: wfctx.ToolRecorder.Calls(),
	}, nil
}

func executeFunctionCall(ctx context.Context, wfctx Context, call client.ToolCall, fallbackHostID string, evidence *domain.FaultEvidence, assessment *domain.FaultAssessment) (any, error) {
	hostID, err := functionHostID(call.Function.Arguments, fallbackHostID)
	if err != nil {
		return nil, err
	}
	switch call.Function.Name {
	case tool.GetHostToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.GetHostToolName, tool.GetHostInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		evidence.Host = out.(domain.Host)
		return evidence.Host, nil
	case tool.QueryMetricsToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryMetricsToolName, tool.QueryMetricsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		evidence.Metrics = out.(domain.HostMetrics)
		return evidence.Metrics, nil
	case tool.QueryAlarmsToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryAlarmsToolName, tool.QueryAlarmsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		evidence.Alarms = out.([]domain.Alarm)
		return evidence.Alarms, nil
	case tool.QueryChangesToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryChangesToolName, tool.QueryChangesInput{HostID: hostID, Since: time.Now().Add(-1 * time.Hour)}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		evidence.RecentChanges = out.([]domain.ChangeRecord)
		return evidence.RecentChanges, nil
	case tool.QueryCMDBToolName:
		out, err := wfctx.Tools.Execute(ctx, tool.QueryCMDBToolName, tool.QueryCMDBInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return nil, err
		}
		evidence.CMDB = out.(map[string]string)
		return evidence.CMDB, nil
	case assessFaultFunctionName:
		*assessment = wfctx.Analyzer.Analyze(*evidence)
		return *assessment, nil
	default:
		return nil, fmt.Errorf("unsupported function call %q", call.Function.Name)
	}
}

func functionHostID(arguments, fallback string) (string, error) {
	if arguments == "" {
		return fallback, nil
	}
	var args struct {
		HostID string `json:"host_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("decode function arguments: %w", err)
	}
	if args.HostID == "" {
		return fallback, nil
	}
	return args.HostID, nil
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

func investigationFunctionTools() []client.FunctionTool {
	hostIDSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host_id": map[string]any{"type": "string", "description": "Host ID, for example host-001"},
		},
		"required": []string{"host_id"},
	}
	return []client.FunctionTool{
		functionTool(tool.GetHostToolName, "查询单台主机基础信息、可达性和健康检查状态", hostIDSchema),
		functionTool(tool.QueryMetricsToolName, "查询单台主机 CPU、内存和持续高负载指标", hostIDSchema),
		functionTool(tool.QueryAlarmsToolName, "查询单台主机当前活跃告警", hostIDSchema),
		functionTool(tool.QueryChangesToolName, "查询单台主机最近一小时变更", hostIDSchema),
		functionTool(tool.QueryCMDBToolName, "查询单台主机 CMDB 元数据", hostIDSchema),
		functionTool(assessFaultFunctionName, "基于已收集证据执行确定性故障评分", hostIDSchema),
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
排查单台主机时，优先调用 get_host、query_metrics、query_alarms、query_changes、query_cmdb，然后调用 assess_fault 获取确定性评分。
最终回答要简短，说明故障等级、关键证据和可能根因。评分必须以 assess_fault 返回为准。`
}
