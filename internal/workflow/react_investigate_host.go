package workflow

import (
	"context"
	"fmt"
	"time"

	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

type ReactTrace struct {
	Step        int    `json:"step"`
	Thought     string `json:"thought"`
	Action      string `json:"action,omitempty"`
	Observation string `json:"observation,omitempty"`
}

type ReactInvestigationResult struct {
	Trace      []ReactTrace           `json:"react_trace"`
	Assessment domain.FaultAssessment `json:"assessment"`
}

type ReactInvestigateHostWorkflow struct{}

func (w ReactInvestigateHostWorkflow) Name() string { return "react_investigate_host" }

func (w ReactInvestigateHostWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	steps := []Step{}
	warnings := []string{}
	trace := []ReactTrace{}
	since := time.Now().Add(-1 * time.Hour)
	var hostID string
	var evidence domain.FaultEvidence
	var assessment domain.FaultAssessment

	addTrace := func(thought, action, observation string) {
		trace = append(trace, ReactTrace{
			Step:        len(trace) + 1,
			Thought:     thought,
			Action:      action,
			Observation: observation,
		})
	}

	if err := runStep(&steps, "react_extract_host_id", func() error {
		var err error
		hostID, err = requireHostID(wfctx.Intent.Parameters)
		if err == nil {
			addTrace("用户要求排查单台机器，先确认目标 host。", "extract_host_id", hostID)
		}
		return err
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "react_action_get_host", func() error {
		addTrace("需要先获取主机基础状态，判断是否可达以及健康检查是否通过。", tool.GetHostToolName, "")
		out, err := wfctx.Tools.Execute(ctx, tool.GetHostToolName, tool.GetHostInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.Host = out.(domain.Host)
		addTrace("主机基础信息已返回。", "", fmt.Sprintf("reachable=%t health_check_passed=%t region=%s env=%s", evidence.Host.Reachable, evidence.Host.HealthCheckPassed, evidence.Host.Region, evidence.Host.Environment))
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "react_action_query_metrics", func() error {
		addTrace("基础状态不足以判断故障，需要观察 CPU、内存和持续高负载。", tool.QueryMetricsToolName, "")
		out, err := wfctx.Tools.Execute(ctx, tool.QueryMetricsToolName, tool.QueryMetricsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.Metrics = out.(domain.HostMetrics)
		addTrace("指标已返回。", "", fmt.Sprintf("cpu=%.0f memory=%.0f high_cpu_minutes=%d", evidence.Metrics.CPUUsagePercent, evidence.Metrics.MemoryUsagePercent, evidence.Metrics.HighCPUDurationMinutes))
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "react_action_query_alarms", func() error {
		addTrace("指标异常需要结合当前告警确认影响面和严重性。", tool.QueryAlarmsToolName, "")
		out, err := wfctx.Tools.Execute(ctx, tool.QueryAlarmsToolName, tool.QueryAlarmsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.Alarms = out.([]domain.Alarm)
		addTrace("告警已返回。", "", fmt.Sprintf("active_alarms=%d", len(evidence.Alarms)))
		return nil
	}); err != nil {
		return Result{}, err
	}

	needChanges := !evidence.Host.Reachable ||
		!evidence.Host.HealthCheckPassed ||
		evidence.Metrics.CPUUsagePercent >= 85 ||
		evidence.Metrics.MemoryUsagePercent >= 95 ||
		len(evidence.Alarms) > 0
	if needChanges {
		if err := runStep(&steps, "react_action_query_changes", func() error {
			addTrace("已有异常信号，继续检查最近一小时变更以辅助根因判断。", tool.QueryChangesToolName, "")
			out, err := wfctx.Tools.Execute(ctx, tool.QueryChangesToolName, tool.QueryChangesInput{HostID: hostID, Since: since}, wfctx.ToolRecorder)
			if err != nil {
				return err
			}
			evidence.RecentChanges = out.([]domain.ChangeRecord)
			addTrace("变更记录已返回。", "", fmt.Sprintf("recent_changes=%d", len(evidence.RecentChanges)))
			return nil
		}); err != nil {
			return Result{}, err
		}
	} else {
		addTrace("主机暂未出现明显异常信号，跳过近期变更查询以减少工具调用。", "skip_query_changes", "no_anomaly_signal")
	}

	if err := runStep(&steps, "react_action_query_cmdb", func() error {
		addTrace("需要补充 CMDB 归属信息，便于输出可执行的排查结论。", tool.QueryCMDBToolName, "")
		out, err := wfctx.Tools.Execute(ctx, tool.QueryCMDBToolName, tool.QueryCMDBInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.CMDB = out.(map[string]string)
		addTrace("CMDB 信息已返回。", "", fmt.Sprintf("owner=%s service=%s", evidence.CMDB["owner"], evidence.CMDB["service"]))
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "react_assess_fault", func() error {
		addTrace("证据收集完成，交给确定性评分器判断故障等级，而不是让 LLM 打分。", "fault_analyzer", "")
		assessment = wfctx.Analyzer.Analyze(evidence)
		addTrace("评分完成。", "", fmt.Sprintf("score=%d level=%s is_faulty=%t", assessment.Score, assessment.Level, assessment.IsFaulty))
		return nil
	}); err != nil {
		return Result{}, err
	}

	summary := ""
	if err := runStep(&steps, "react_generate_diagnosis", func() error {
		var err error
		summary, err = wfctx.LLM.GenerateHostDiagnosis(ctx, assessment)
		if err != nil {
			summary = "ReAct 排查摘要生成失败，已返回确定性评分结果。"
			warnings = append(warnings, "llm diagnosis failed; used fallback summary")
			return nil
		}
		return nil
	}); err != nil {
		return Result{}, err
	}

	return Result{
		Intent:   wfctx.Intent.Name,
		Workflow: w.Name(),
		Summary:  summary,
		Results: ReactInvestigationResult{
			Trace:      trace,
			Assessment: assessment,
		},
		Warnings:  warnings,
		Steps:     steps,
		ToolCalls: wfctx.ToolRecorder.Calls(),
	}, nil
}
