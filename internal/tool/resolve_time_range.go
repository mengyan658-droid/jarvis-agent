package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"jarvis-agent/internal/domain"
)

const ResolveTimeRangeToolName = "resolve_time_range"

type ResolveTimeRangeInput struct {
	Kind      string `json:"kind"`
	Amount    int    `json:"amount,omitempty"`
	Unit      string `json:"unit,omitempty"`
	StartText string `json:"start_text,omitempty"`
	EndText   string `json:"end_text,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
}

type ResolveTimeRangeTool struct {
	Now             func() time.Time
	DefaultLocation *time.Location
	DefaultLookback time.Duration
	MaxRange        time.Duration
}

func (t ResolveTimeRangeTool) Name() string { return ResolveTimeRangeToolName }

func (t ResolveTimeRangeTool) Execute(ctx context.Context, input any) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	in, err := normalizeResolveTimeRangeInput(input)
	if err != nil {
		return nil, err
	}
	in, err = normalizeResolveTimeRangeFields(in)
	if err != nil {
		return nil, err
	}
	if err := validateResolveTimeRangeInput(in); err != nil {
		return nil, err
	}

	now := t.now()
	loc, err := resolveLocation(in.Timezone, t.DefaultLocation)
	if err != nil {
		return nil, err
	}
	now = now.In(loc)

	start, end, source, isDefault, err := t.resolve(in, now, loc)
	if err != nil {
		return nil, err
	}
	if err := validateResolvedTimeRange(start, end, now, t.maxRange()); err != nil {
		return nil, err
	}

	return domain.NewTimeRange(start, end, now, loc.String(), source, isDefault), nil
}

func (t ResolveTimeRangeTool) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (t ResolveTimeRangeTool) defaultLookback() time.Duration {
	if t.DefaultLookback > 0 {
		return t.DefaultLookback
	}
	return time.Hour
}

func (t ResolveTimeRangeTool) maxRange() time.Duration {
	if t.MaxRange > 0 {
		return t.MaxRange
	}
	return 7 * 24 * time.Hour
}

func (t ResolveTimeRangeTool) resolve(in ResolveTimeRangeInput, now time.Time, loc *time.Location) (time.Time, time.Time, string, bool, error) {
	switch in.Kind {
	case "default":
		return now.Add(-t.defaultLookback()), now, "default", true, nil
	case "today":
		start := startOfDay(now, loc)
		return start, now, "today", false, nil
	case "yesterday":
		today := startOfDay(now, loc)
		return today.AddDate(0, 0, -1), today, "yesterday", false, nil
	case "relative":
		duration, err := lookbackDuration(in.Amount, in.Unit)
		if err != nil {
			return time.Time{}, time.Time{}, "", false, err
		}
		return now.Add(-duration), now, "relative", false, nil
	case "since":
		if strings.TrimSpace(in.StartText) == "" {
			return time.Time{}, time.Time{}, "", false, fmt.Errorf("start_text is required for since time range")
		}
		start, err := parseTimeText(in.StartText, now, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", false, fmt.Errorf("parse start_text: %w", err)
		}
		return start, now, "since", false, nil
	case "absolute_range":
		if strings.TrimSpace(in.StartText) == "" || strings.TrimSpace(in.EndText) == "" {
			return time.Time{}, time.Time{}, "", false, fmt.Errorf("start_text and end_text are required for absolute_range")
		}
		start, err := parseTimeText(in.StartText, now, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", false, fmt.Errorf("parse start_text: %w", err)
		}
		end, err := parseTimeText(in.EndText, now, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", false, fmt.Errorf("parse end_text: %w", err)
		}
		return start, end, "absolute_range", false, nil
	default:
		return time.Time{}, time.Time{}, "", false, fmt.Errorf("unsupported time range kind %q", in.Kind)
	}
}

func normalizeResolveTimeRangeInput(input any) (ResolveTimeRangeInput, error) {
	switch v := input.(type) {
	case ResolveTimeRangeInput:
		return v, nil
	case *ResolveTimeRangeInput:
		if v == nil {
			return ResolveTimeRangeInput{}, fmt.Errorf("resolve time range input is nil")
		}
		return *v, nil
	case map[string]any:
		return ResolveTimeRangeInput{
			Kind:      stringValue(v["kind"]),
			Amount:    intValue(v["amount"]),
			Unit:      stringValue(v["unit"]),
			StartText: stringValue(v["start_text"]),
			EndText:   stringValue(v["end_text"]),
			Timezone:  stringValue(v["timezone"]),
		}, nil
	default:
		return ResolveTimeRangeInput{}, fmt.Errorf("invalid resolve time range input %T", input)
	}
}

func normalizeResolveTimeRangeFields(in ResolveTimeRangeInput) (ResolveTimeRangeInput, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		kind = "default"
	}
	switch kind {
	case "default":
		in.Kind = "default"
	case "today":
		in.Kind = "today"
	case "yesterday":
		in.Kind = "yesterday"
	case "relative", "last", "lookback":
		in.Kind = "relative"
	case "since":
		in.Kind = "since"
	case "absolute_range", "absolute":
		in.Kind = "absolute_range"
	default:
		return ResolveTimeRangeInput{}, fmt.Errorf("unsupported time range kind %q", in.Kind)
	}

	unit, err := canonicalTimeUnit(in.Unit)
	if err != nil {
		return ResolveTimeRangeInput{}, err
	}
	in.Unit = unit
	in.StartText = strings.TrimSpace(in.StartText)
	in.EndText = strings.TrimSpace(in.EndText)
	in.Timezone = strings.TrimSpace(in.Timezone)
	return in, nil
}

func validateResolveTimeRangeInput(in ResolveTimeRangeInput) error {
	hasAmountOrUnit := in.Amount != 0 || in.Unit != ""
	hasStartOrEnd := in.StartText != "" || in.EndText != ""
	switch in.Kind {
	case "default", "today", "yesterday":
		if hasAmountOrUnit || hasStartOrEnd {
			return fmt.Errorf("%s time range must not include amount, unit, start_text or end_text", in.Kind)
		}
	case "relative":
		if in.Amount <= 0 {
			return fmt.Errorf("amount must be a positive integer for relative time range")
		}
		if in.Unit == "" {
			return fmt.Errorf("unit is required for relative time range")
		}
		if hasStartOrEnd {
			return fmt.Errorf("relative time range must not include start_text or end_text")
		}
	case "since":
		if in.StartText == "" {
			return fmt.Errorf("start_text is required for since time range")
		}
		if in.EndText != "" || hasAmountOrUnit {
			return fmt.Errorf("since time range must not include end_text, amount or unit")
		}
	case "absolute_range":
		if in.StartText == "" || in.EndText == "" {
			return fmt.Errorf("start_text and end_text are required for absolute_range")
		}
		if hasAmountOrUnit {
			return fmt.Errorf("absolute_range must not include amount or unit")
		}
	default:
		return fmt.Errorf("unsupported time range kind %q", in.Kind)
	}
	return nil
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intValue(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		if math.Trunc(n) == n {
			return int(n)
		}
	case json.Number:
		i, _ := strconv.Atoi(n.String())
		return i
	}
	return 0
}

func resolveLocation(name string, fallback *time.Location) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if fallback != nil {
			return fallback, nil
		}
		name = "Asia/Shanghai"
	}
	switch strings.ToLower(name) {
	case "default", "asia/shanghai", "utc+8", "utc+08:00", "gmt+8", "gmt+08:00", "+08:00", "cst":
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err == nil {
			return loc, nil
		}
		return time.FixedZone("Asia/Shanghai", 8*3600), nil
	}
	loc, err := time.LoadLocation(name)
	if err == nil {
		return loc, nil
	}
	if name == "Asia/Shanghai" {
		return time.FixedZone("Asia/Shanghai", 8*3600), nil
	}
	return nil, fmt.Errorf("load timezone %q: %w", name, err)
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func lookbackDuration(amount int, unit string) (time.Duration, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be a positive integer")
	}
	switch unit {
	case "minute":
		return time.Duration(amount) * time.Minute, nil
	case "hour":
		return time.Duration(amount) * time.Hour, nil
	case "day":
		return time.Duration(amount) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported time unit %q", unit)
	}
}

func canonicalTimeUnit(unit string) (string, error) {
	unit = strings.ToLower(strings.TrimSpace(unit))
	if unit == "" {
		return "", nil
	}
	switch unit {
	case "m", "min", "minute", "minutes":
		return "minute", nil
	case "h", "hour", "hours":
		return "hour", nil
	case "d", "day", "days":
		return "day", nil
	default:
		return "", fmt.Errorf("unsupported time unit %q", unit)
	}
}

func validateResolvedTimeRange(start, end, now time.Time, maxRange time.Duration) error {
	if start.IsZero() || end.IsZero() {
		return fmt.Errorf("time range start and end must not be zero")
	}
	if !start.Before(end) {
		return fmt.Errorf("time range start must be before end")
	}
	if start.After(now) || end.After(now) {
		return fmt.Errorf("time range must not be in the future")
	}
	if maxRange > 0 && end.Sub(start) > maxRange {
		return fmt.Errorf("time range exceeds max range %s", maxRange)
	}
	return nil
}

func parseTimeText(text string, now time.Time, loc *time.Location) (time.Time, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return time.Time{}, fmt.Errorf("time text is empty")
	}
	lower := strings.ToLower(raw)
	if lower == "now" || raw == "现在" {
		return now.In(loc), nil
	}

	base := startOfDay(now, loc)
	hasDayAlias := false
	cleaned := raw
	switch {
	case strings.Contains(cleaned, "前天"):
		base = base.AddDate(0, 0, -2)
		cleaned = strings.ReplaceAll(cleaned, "前天", "")
		hasDayAlias = true
	case strings.Contains(cleaned, "昨天"):
		base = base.AddDate(0, 0, -1)
		cleaned = strings.ReplaceAll(cleaned, "昨天", "")
		hasDayAlias = true
	case strings.Contains(cleaned, "今天"):
		cleaned = strings.ReplaceAll(cleaned, "今天", "")
		hasDayAlias = true
	case strings.Contains(cleaned, "明天"):
		base = base.AddDate(0, 0, 1)
		cleaned = strings.ReplaceAll(cleaned, "明天", "")
		hasDayAlias = true
	}
	cleaned = strings.TrimSpace(cleaned)
	if hasDayAlias && cleaned == "" {
		return base, nil
	}

	if t, ok := parseAbsoluteTime(cleaned, loc); ok {
		return t, nil
	}
	if t, ok := parseTimeOfDay(cleaned, base, loc); ok {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unsupported time text %q", text)
}

func parseAbsoluteTime(text string, loc *time.Location) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return t.In(loc), true
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, text, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseTimeOfDay(text string, base time.Time, loc *time.Location) (time.Time, bool) {
	s, period := normalizeTimeOfDayText(text)
	if s == "" {
		return time.Time{}, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, false
	}
	minute := 0
	second := 0
	if len(parts) >= 2 && parts[1] != "" {
		minute, err = strconv.Atoi(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}
	if len(parts) == 3 && parts[2] != "" {
		second, err = strconv.Atoi(parts[2])
		if err != nil {
			return time.Time{}, false
		}
	}
	if period == "pm" && hour < 12 {
		hour += 12
	}
	if period == "noon" && hour < 11 {
		hour += 12
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return time.Time{}, false
	}
	return time.Date(base.Year(), base.Month(), base.Day(), hour, minute, second, 0, loc), true
}

func normalizeTimeOfDayText(text string) (string, string) {
	s := strings.TrimSpace(text)
	period := ""
	replacements := map[string]string{
		"上午": "",
		"早上": "",
		"凌晨": "",
		"下午": "",
		"晚上": "",
		"晚间": "",
		"中午": "",
	}
	for marker, replacement := range replacements {
		if strings.Contains(s, marker) {
			switch marker {
			case "下午", "晚上", "晚间":
				period = "pm"
			case "中午":
				period = "noon"
			}
			s = strings.ReplaceAll(s, marker, replacement)
		}
	}
	s = strings.ReplaceAll(s, "：", ":")
	s = strings.ReplaceAll(s, "点半", ":30")
	s = strings.ReplaceAll(s, "点钟", ":")
	s = strings.ReplaceAll(s, "点", ":")
	s = strings.ReplaceAll(s, "时", ":")
	s = strings.ReplaceAll(s, "分", "")
	s = strings.ReplaceAll(s, "秒", "")
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, ":") {
		s += "00"
	}
	return s, period
}
