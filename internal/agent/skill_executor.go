package agent

import (
	"context"
	"fmt"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/skill"
	"jarvis-agent/internal/tool"
	"jarvis-agent/internal/workflow"
)

func (r *Runtime) executeSkill(ctx context.Context, spec skill.Spec, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, []string, error) {
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	intent.Name = spec.Name

	switch spec.ExecutorOrDefault() {
	case skill.ExecutorWorkflow:
		return r.executeWorkflowSkill(ctx, spec, intent, message, recorder)
	case skill.ExecutorToolLoop:
		return r.executeToolLoopSkill(ctx, spec, intent, message, recorder)
	case skill.ExecutorGuidedSteps:
		return r.executeGuidedStepsSkill(ctx, spec, intent, message, recorder)
	case skill.ExecutorSubAgent:
		return workflow.Result{}, nil, fmt.Errorf("skill %q executor %q is declared but not implemented", spec.Name, skill.ExecutorSubAgent)
	default:
		return workflow.Result{}, nil, fmt.Errorf("skill %q unsupported executor %q", spec.Name, spec.ExecutorOrDefault())
	}
}

func (r *Runtime) executeWorkflowSkill(ctx context.Context, spec skill.Spec, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, []string, error) {
	if spec.Workflow == "" {
		return workflow.Result{}, nil, fmt.Errorf("skill %q workflow is required", spec.Name)
	}
	result, err := r.runWorkflow(ctx, spec.Workflow, intent, message, recorder)
	return result, nil, err
}

func (r *Runtime) executeToolLoopSkill(ctx context.Context, spec skill.Spec, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, []string, error) {
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	if intent.Parameters["host_id"] == "" {
		if hostID := extractHostID(message); hostID != "" {
			intent.Parameters["host_id"] = hostID
		}
	}
	target := spec.Workflow
	if target == "" {
		target = spec.Name
	}
	result, err := r.runWorkflow(ctx, target, intent, message, recorder)
	return result, nil, err
}

func (r *Runtime) executeGuidedStepsSkill(ctx context.Context, spec skill.Spec, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, []string, error) {
	target := spec.Workflow
	if target == "" {
		target = spec.Name
	}
	result, err := r.runWorkflow(ctx, target, intent, message, recorder)
	return result, nil, err
}

func (r *Runtime) executeWorkflowFallback(ctx context.Context, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, []string, error) {
	warnings := []string{}
	routeName := intent.Name
	if routeName == "" || routeName == "unknown" {
		warnings = append(warnings, "intent is unknown; routed to tool loop workflow")
		routeName = workflow.ToolLoopInvestigateHostWorkflowName
	}

	if _, err := r.Workflows.Get(routeName); err != nil {
		warnings = append(warnings, "intent workflow not found; routed to tool loop workflow")
		routeName = workflow.ToolLoopInvestigateHostWorkflowName
	}
	if routeName == workflow.ToolLoopInvestigateHostWorkflowName {
		if intent.Parameters == nil {
			intent.Parameters = map[string]string{}
		}
		if intent.Parameters["host_id"] == "" {
			if hostID := extractHostID(message); hostID != "" {
				intent.Parameters["host_id"] = hostID
			}
		}
	}
	intent.Name = routeName
	result, err := r.runWorkflow(ctx, routeName, intent, message, recorder)
	return result, warnings, err
}

func (r *Runtime) runWorkflow(ctx context.Context, name string, intent client.Intent, message string, recorder *tool.Recorder) (workflow.Result, error) {
	wf, err := r.Workflows.Get(name)
	if err != nil {
		return workflow.Result{}, err
	}
	return wf.Run(ctx, workflow.Context{
		Intent:       intent,
		Message:      message,
		Tools:        r.Tools,
		ToolRecorder: recorder,
		Analyzer:     r.Analyzer,
		LLM:          r.LLM,
		MaxSteps:     r.MaxSteps,
	})
}
