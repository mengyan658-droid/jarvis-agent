package workflow

import (
	"context"
	"time"

	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

type DiagnoseHostWorkflow struct{}

func (w DiagnoseHostWorkflow) Name() string { return "diagnose_host" }

func (w DiagnoseHostWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	steps := []Step{}
	warnings := []string{}
	since := time.Now().Add(-1 * time.Hour)
	var hostID string
	var evidence domain.FaultEvidence
	var assessment domain.FaultAssessment

	if err := runStep(&steps, "extract_host_id", func() error {
		var err error
		hostID, err = requireHostID(wfctx.Intent.Parameters)
		return err
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "query_host", func() error {
		out, err := wfctx.Tools.Execute(ctx, tool.GetHostToolName, tool.GetHostInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.Host = out.(domain.Host)
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "query_metrics_and_alarms", func() error {
		metricsOut, err := wfctx.Tools.Execute(ctx, tool.QueryMetricsToolName, tool.QueryMetricsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		alarmsOut, err := wfctx.Tools.Execute(ctx, tool.QueryAlarmsToolName, tool.QueryAlarmsInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.Metrics = metricsOut.(domain.HostMetrics)
		evidence.Alarms = alarmsOut.([]domain.Alarm)
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "query_recent_changes", func() error {
		out, err := wfctx.Tools.Execute(ctx, tool.QueryChangesToolName, tool.QueryChangesInput{HostID: hostID, Since: since}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.RecentChanges = out.([]domain.ChangeRecord)
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "query_cmdb", func() error {
		out, err := wfctx.Tools.Execute(ctx, tool.QueryCMDBToolName, tool.QueryCMDBInput{HostID: hostID}, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		evidence.CMDB = out.(map[string]string)
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "assess_fault", func() error {
		assessment = wfctx.Analyzer.Analyze(evidence)
		return nil
	}); err != nil {
		return Result{}, err
	}

	summary := ""
	if err := runStep(&steps, "generate_diagnosis", func() error {
		var err error
		summary, err = wfctx.LLM.GenerateHostDiagnosis(ctx, assessment)
		if err != nil {
			summary = "诊断摘要生成失败，已返回确定性评分结果。"
			warnings = append(warnings, "llm diagnosis failed; used fallback summary")
			return nil
		}
		return nil
	}); err != nil {
		return Result{}, err
	}

	return Result{Intent: wfctx.Intent.Name, Workflow: w.Name(), Summary: summary, Results: assessment, Warnings: warnings, Steps: steps, ToolCalls: wfctx.ToolRecorder.Calls()}, nil
}
