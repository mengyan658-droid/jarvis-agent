package agent

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/skill"
	"jarvis-agent/internal/tool"
	"jarvis-agent/internal/workflow"
)

type Runtime struct {
	LLM          client.LLMClient
	Tools        *tool.Registry
	Skills       *skill.Registry
	Workflows    *workflow.Registry
	Analyzer     *domain.FaultAnalyzer
	Timeout      time.Duration
	MaxSteps     int
	MaxToolCalls int
}

type QueryResult struct {
	RequestID  string            `json:"request_id"`
	Intent     string            `json:"intent"`
	Workflow   string            `json:"workflow"`
	Summary    string            `json:"summary"`
	Results    any               `json:"results"`
	Warnings   []string          `json:"warnings,omitempty"`
	Steps      []workflow.Step   `json:"execution_steps"`
	ToolCalls  []tool.CallRecord `json:"tool_calls"`
	DurationMS int64             `json:"duration_ms"`
}

func (r *Runtime) Query(ctx context.Context, requestID, message string) (QueryResult, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	ctx = tool.ContextWithRequestID(ctx, requestID)

	intent, routeWarnings, err := r.selectIntent(ctx, message)
	if err != nil {
		return QueryResult{}, err
	}
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	if startText, endText, ok := extractAbsoluteTimeRange(message); ok {
		intent.Parameters["start_text"] = startText
		intent.Parameters["end_text"] = endText
		intent.Parameters["since"] = ""
	} else if intent.Parameters["since"] == "" {
		if since := extractSince(message); since != "" {
			intent.Parameters["since"] = since
		}
	}
	routeName := intent.Name
	if spec, ok := r.skillForIntent(routeName); ok {
		routeName = spec.Workflow
		intent.Name = spec.Workflow
	}
	if routeName == "" || routeName == "unknown" {
		routeWarnings = append(routeWarnings, "intent is unknown; routed to tool loop workflow")
		routeName = workflow.ToolLoopInvestigateHostWorkflowName
	}
	wf, err := r.Workflows.Get(routeName)
	if err != nil {
		routeWarnings = append(routeWarnings, "intent workflow not found; routed to tool loop workflow")
		routeName = workflow.ToolLoopInvestigateHostWorkflowName
		wf, err = r.Workflows.Get(routeName)
		if err != nil {
			return QueryResult{}, err
		}
	}
	if routeName == workflow.ToolLoopInvestigateHostWorkflowName {
		intent.Name = routeName
		if intent.Parameters == nil {
			intent.Parameters = map[string]string{}
		}
		if intent.Parameters["host_id"] == "" {
			if hostID := extractHostID(message); hostID != "" {
				intent.Parameters["host_id"] = hostID
			}
		}
	}
	recorder := &tool.Recorder{}
	result, err := wf.Run(ctx, workflow.Context{
		Intent:       intent,
		Tools:        r.Tools,
		ToolRecorder: recorder,
		Analyzer:     r.Analyzer,
		LLM:          r.LLM,
		MaxSteps:     r.MaxSteps,
	})
	if err != nil {
		return QueryResult{}, err
	}
	result.Warnings = append(routeWarnings, result.Warnings...)
	if r.MaxToolCalls > 0 && len(result.ToolCalls) > r.MaxToolCalls {
		result.Warnings = append(result.Warnings, "tool call count exceeded configured limit")
	}
	return QueryResult{
		RequestID:  requestID,
		Intent:     result.Intent,
		Workflow:   result.Workflow,
		Summary:    result.Summary,
		Results:    result.Results,
		Warnings:   result.Warnings,
		Steps:      result.Steps,
		ToolCalls:  result.ToolCalls,
		DurationMS: time.Since(started).Milliseconds(),
	}, nil
}

func extractHostID(message string) string {
	return regexp.MustCompile(`host-\d{3}`).FindString(message)
}

func (r *Runtime) selectIntent(ctx context.Context, message string) (client.Intent, []string, error) {
	warnings := []string{}
	if r.Skills == nil || len(r.Skills.Names()) == 0 {
		intent, err := r.LLM.ParseIntent(ctx, message)
		return intent, warnings, err
	}
	functionLLM, ok := r.LLM.(client.FunctionCallingClient)
	if !ok {
		intent, err := r.LLM.ParseIntent(ctx, message)
		return intent, warnings, err
	}

	assistant, err := functionLLM.ChatWithTools(ctx, []client.ToolChatMessage{
		{Role: "system", Content: skill.RouterSystemPrompt(r.Skills)},
		{Role: "user", Content: message},
	}, []client.FunctionTool{skill.SelectSkillFunctionTool(r.Skills)})
	if err != nil {
		warnings = append(warnings, "skill router failed; used intent parser")
		intent, parseErr := r.LLM.ParseIntent(ctx, message)
		return intent, warnings, parseErr
	}
	for _, call := range assistant.ToolCalls {
		if call.Function.Name != skill.SelectSkillFunctionName {
			continue
		}
		selection, err := skill.DecodeSelection(call.Function.Arguments)
		if err != nil {
			warnings = append(warnings, "skill router returned invalid arguments; used intent parser")
			break
		}
		if _, ok := r.Skills.Get(selection.Skill); !ok {
			warnings = append(warnings, "skill router returned unknown skill; used intent parser")
			break
		}
		return client.Intent{Name: selection.Skill, Parameters: selection.Parameters}, warnings, nil
	}
	warnings = append(warnings, "skill router returned no skill; used intent parser")
	intent, err := r.LLM.ParseIntent(ctx, message)
	return intent, warnings, err
}

func (r *Runtime) skillForIntent(name string) (skill.Spec, bool) {
	if r.Skills == nil {
		return skill.Spec{}, false
	}
	if spec, ok := r.Skills.Get(name); ok {
		return spec, true
	}
	return r.Skills.GetByIntent(name)
}

func extractAbsoluteTimeRange(message string) (string, string, bool) {
	dateExpr := `(?:(?:\d{4}年)?\d{1,2}月\d{1,2}[日号]?(?:\s*(?:上午|下午|晚上|晚间|中午|凌晨|早上)?\s*\d{1,2}(?:[:：点时]\d{0,2})?(?:分)?)?|(?:\d{4}[-/])?\d{1,2}[-/]\d{1,2}(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?)`
	pattern := `(` + dateExpr + `)\s*(?:到|至|~|～|—|--)\s*(` + dateExpr + `)`
	matches := regexp.MustCompile(pattern).FindStringSubmatch(message)
	if len(matches) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
}

func extractSince(message string) string {
	s := strings.TrimSpace(message)
	switch {
	case strings.Contains(s, "今天"):
		return "today"
	case strings.Contains(s, "昨天"):
		return "yesterday"
	case strings.Contains(s, "近一周") || strings.Contains(s, "最近一周") || strings.Contains(s, "过去一周"):
		return "1w"
	}
	if since := extractCompactRelativeSince(s); since != "" {
		return since
	}
	return extractChineseRelativeSince(s)
}

func extractCompactRelativeSince(message string) string {
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

func extractChineseRelativeSince(message string) string {
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
