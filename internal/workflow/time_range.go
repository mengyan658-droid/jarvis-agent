package workflow

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"jarvis-agent/internal/domain"
	"jarvis-agent/internal/tool"
)

func resolveWorkflowTimeRange(ctx context.Context, wfctx Context, steps *[]Step) (domain.TimeRange, error) {
	var timeRange domain.TimeRange
	err := runStep(steps, "resolve_time_range", func() error {
		out, err := wfctx.Tools.Execute(ctx, tool.ResolveTimeRangeToolName, timeRangeInputFromParameters(wfctx.Intent.Parameters), wfctx.ToolRecorder)
		if err != nil {
			return err
		}
		timeRange = out.(domain.TimeRange)
		return nil
	})
	return timeRange, err
}

func timeRangeInputFromParameters(params map[string]string) tool.ResolveTimeRangeInput {
	if params == nil {
		return tool.ResolveTimeRangeInput{Kind: "default"}
	}
	if strings.TrimSpace(params["start_text"]) != "" || strings.TrimSpace(params["end_text"]) != "" {
		return tool.ResolveTimeRangeInput{
			Kind:      "absolute_range",
			StartText: params["start_text"],
			EndText:   params["end_text"],
		}
	}
	return timeRangeInputFromText(params["since"])
}

func timeRangeInputFromText(text string) tool.ResolveTimeRangeInput {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return tool.ResolveTimeRangeInput{Kind: "default"}
	}
	switch s {
	case "today", "今天":
		return tool.ResolveTimeRangeInput{Kind: "today"}
	case "yesterday", "昨天":
		return tool.ResolveTimeRangeInput{Kind: "yesterday"}
	case "last_week", "recent_week", "近一周", "最近一周", "过去一周":
		return tool.ResolveTimeRangeInput{Kind: "relative", Amount: 1, Unit: "week"}
	}
	if in, ok := parseCompactRelativeTime(s); ok {
		return in
	}
	if in, ok := parseChineseRelativeTime(s); ok {
		return in
	}
	return tool.ResolveTimeRangeInput{Kind: "default"}
}

func parseCompactRelativeTime(s string) (tool.ResolveTimeRangeInput, bool) {
	matches := regexp.MustCompile(`^(\d+)\s*([mhdw])$`).FindStringSubmatch(s)
	if len(matches) != 3 {
		return tool.ResolveTimeRangeInput{}, false
	}
	amount, err := strconv.Atoi(matches[1])
	if err != nil || amount <= 0 {
		return tool.ResolveTimeRangeInput{}, false
	}
	unit := map[string]string{"m": "minute", "h": "hour", "d": "day", "w": "week"}[matches[2]]
	return tool.ResolveTimeRangeInput{Kind: "relative", Amount: amount, Unit: unit}, true
}

func parseChineseRelativeTime(s string) (tool.ResolveTimeRangeInput, bool) {
	if !(strings.Contains(s, "最近") || strings.Contains(s, "近") || strings.Contains(s, "过去")) {
		return tool.ResolveTimeRangeInput{}, false
	}
	matches := regexp.MustCompile(`([0-9一二两三四五六七八九十]+)\s*(分钟|小时|天|日|周|星期)`).FindStringSubmatch(s)
	if len(matches) != 3 {
		return tool.ResolveTimeRangeInput{}, false
	}
	amount, ok := parseSmallChineseNumber(matches[1])
	if !ok || amount <= 0 {
		return tool.ResolveTimeRangeInput{}, false
	}
	unit := "day"
	switch matches[2] {
	case "分钟":
		unit = "minute"
	case "小时":
		unit = "hour"
	case "周", "星期":
		unit = "week"
	}
	return tool.ResolveTimeRangeInput{Kind: "relative", Amount: amount, Unit: unit}, true
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
