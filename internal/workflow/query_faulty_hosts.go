package workflow

import (
	"context"
	"errors"
	"sort"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

type QueryFaultyHostsWorkflow struct{}

func (w QueryFaultyHostsWorkflow) Name() string { return "query_faulty_hosts" }

func (w QueryFaultyHostsWorkflow) Run(ctx context.Context, wfctx Context) (Result, error) {
	steps := []Step{}
	warnings := []string{}
	params := wfctx.Intent.Parameters
	var query client.HostQuery
	var timeRange domain.TimeRange

	if err := runStep(&steps, "parse_query_parameters", func() error {
		query.Region = params["region"]
		query.Environment = params["environment"]
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := runStep(&steps, "validate_parameters", func() error {
		return nil
	}); err != nil {
		return Result{}, err
	}

	var err error
	timeRange, err = resolveWorkflowTimeRange(ctx, wfctx, &steps)
	if err != nil {
		return Result{}, err
	}

	var hosts []domain.Host
	if err := runStep(&steps, "query_hosts", func() error {
		out, err := wfctx.Tools.Execute(ctx, tool.QueryHostsToolName, query, wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		hosts = out.([]domain.Host)
		return nil
	}); err != nil {
		return Result{}, err
	}

	assessments := make([]domain.FaultAssessment, 0, len(hosts))
	for _, host := range hosts {
		evidence := domain.FaultEvidence{Host: host}
		if err := runStep(&steps, "query_metrics_and_alarms:"+host.ID, func() error {
			metricsOut, err := wfctx.Tools.Execute(ctx, tool.QueryMetricsToolName, tool.QueryMetricsInput{HostID: host.ID}, wfctx.ToolRecorder)
			if err != nil {
				return err
			}
			alarmsOut, err := wfctx.Tools.Execute(ctx, tool.QueryAlarmsToolName, tool.QueryAlarmsInput{HostID: host.ID}, wfctx.ToolRecorder)
			if err != nil {
				return err
			}
			evidence.Metrics = metricsOut.(domain.HostMetrics)
			evidence.Alarms = alarmsOut.([]domain.Alarm)
			return nil
		}); err != nil {
			return Result{}, err
		}
		if err := runStep(&steps, "query_recent_changes:"+host.ID, func() error {
			out, err := wfctx.Tools.Execute(ctx, tool.QueryChangesToolName, tool.QueryChangesInput{HostID: host.ID, TimeRange: timeRange}, wfctx.ToolRecorder)
			if err != nil {
				return err
			}
			evidence.RecentChanges = out.([]domain.ChangeRecord)
			return nil
		}); err != nil {
			return Result{}, err
		}
		if err := runStep(&steps, "query_cmdb:"+host.ID, func() error {
			out, err := wfctx.Tools.Execute(ctx, tool.QueryCMDBToolName, tool.QueryCMDBInput{HostID: host.ID}, wfctx.ToolRecorder)
			if err != nil {
				return err
			}
			evidence.CMDB = out.(map[string]string)
			return nil
		}); err != nil {
			return Result{}, err
		}
		if err := runStep(&steps, "assess_fault:"+host.ID, func() error {
			assessments = append(assessments, wfctx.Analyzer.Analyze(evidence))
			return nil
		}); err != nil {
			return Result{}, err
		}
	}

	if err := runStep(&steps, "sort_and_filter", func() error {
		filtered := assessments[:0]
		for _, assessment := range assessments {
			if assessment.IsFaulty {
				filtered = append(filtered, assessment)
			}
		}
		assessments = filtered
		sort.Slice(assessments, func(i, j int) bool { return assessments[i].Score > assessments[j].Score })
		return nil
	}); err != nil {
		return Result{}, err
	}

	summary := ""
	if err := runStep(&steps, "generate_summary", func() error {
		var err error
		summary, err = wfctx.LLM.GenerateFaultSummary(ctx, assessments)
		if err != nil {
			summary = fallbackFaultSummary(assessments)
			warnings = append(warnings, "llm summary failed; used fallback summary")
			return nil
		}
		return nil
	}); err != nil {
		return Result{}, err
	}

	return Result{Intent: wfctx.Intent.Name, Workflow: w.Name(), Summary: summary, Results: assessments, Warnings: warnings, Steps: steps, ToolCalls: wfctx.ToolRecorder.Calls()}, nil
}

func fallbackFaultSummary(assessments []domain.FaultAssessment) string {
	if len(assessments) == 0 {
		return "未发现故障机器。"
	}
	return "发现故障机器，已按评分倒序返回。"
}

func requireHostID(params map[string]string) (string, error) {
	hostID := params["host_id"]
	if hostID == "" {
		return "", errors.New("host_id is required")
	}
	return hostID, nil
}
