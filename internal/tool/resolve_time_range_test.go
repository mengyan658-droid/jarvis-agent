package tool

import (
	"context"
	"testing"
	"time"

	"jarvis-agent/internal/domain"
)

func TestResolveTimeRangeToolDefaultUsesLastHour(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{}, now, loc)

	wantStart := time.Date(2026, 7, 8, 9, 30, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.Source != "default" || !out.IsDefault {
		t.Fatalf("unexpected source/default: source=%s is_default=%t", out.Source, out.IsDefault)
	}
	if out.StartUnixMS != wantStart.UnixMilli() || out.EndUnixMS != now.UnixMilli() {
		t.Fatalf("unexpected unix millis: %+v", out)
	}
	if out.RangeID == "" || out.Interval != "[start,end)" {
		t.Fatalf("unexpected range metadata: %+v", out)
	}
	if out.DurationSec != 3600 || out.DurationMS != int64(time.Hour/time.Millisecond) {
		t.Fatalf("unexpected duration: %+v", out)
	}
}

func TestResolveTimeRangeToolToday(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{Kind: "today"}, now, loc)

	wantStart := time.Date(2026, 7, 8, 0, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.Source != "today" || out.IsDefault {
		t.Fatalf("unexpected source/default: source=%s is_default=%t", out.Source, out.IsDefault)
	}
}

func TestResolveTimeRangeToolYesterdayUsesNaturalDay(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{Kind: "yesterday"}, now, loc)

	wantStart := time.Date(2026, 7, 7, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 7, 8, 0, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, wantEnd)
	if out.Source != "yesterday" {
		t.Fatalf("unexpected source: %s", out.Source)
	}
}

func TestResolveTimeRangeToolRelative(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:   "relative",
		Amount: 2,
		Unit:   "hour",
	}, now, loc)

	wantStart := time.Date(2026, 7, 8, 8, 30, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.Source != "relative" {
		t.Fatalf("unexpected source: %s", out.Source)
	}
}

func TestResolveTimeRangeToolRelativeTwoDays(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:   "relative",
		Amount: 2,
		Unit:   "day",
	}, now, loc)

	wantStart := time.Date(2026, 7, 6, 10, 30, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.DurationSec != int64(48*time.Hour/time.Second) {
		t.Fatalf("unexpected duration: %+v", out)
	}
}

func TestResolveTimeRangeToolRelativeOneWeek(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:   "relative",
		Amount: 1,
		Unit:   "week",
	}, now, loc)

	wantStart := time.Date(2026, 7, 1, 10, 30, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.DurationSec != int64(7*24*time.Hour/time.Second) {
		t.Fatalf("unexpected duration: %+v", out)
	}
}

func TestResolveTimeRangeToolSinceParsesChineseTime(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:      "since",
		StartText: "今天10点",
	}, now, loc)

	wantStart := time.Date(2026, 7, 8, 10, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
	if out.Source != "since" {
		t.Fatalf("unexpected source: %s", out.Source)
	}
}

func TestResolveTimeRangeToolAbsoluteRange(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:      "absolute_range",
		StartText: "2026-07-08 09:00",
		EndText:   "2026-07-08 10:00",
	}, now, loc)

	wantStart := time.Date(2026, 7, 8, 9, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 7, 8, 10, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, wantEnd)
	if out.Source != "absolute_range" {
		t.Fatalf("unexpected source: %s", out.Source)
	}
}

func TestResolveTimeRangeToolAbsoluteChineseDateRangeIncludesEndDate(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:      "absolute_range",
		StartText: "7月1号",
		EndText:   "7月5号",
	}, now, loc)

	wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 7, 6, 0, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, wantEnd)
	if out.DurationSec != int64(5*24*time.Hour/time.Second) {
		t.Fatalf("unexpected duration: %+v", out)
	}
}

func TestResolveTimeRangeToolAbsoluteDateRangeWithTimeKeepsExactEnd(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:      "absolute_range",
		StartText: "7月1号 10点",
		EndText:   "7月5号 18点",
	}, now, loc)

	wantStart := time.Date(2026, 7, 1, 10, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 7, 5, 18, 0, 0, 0, loc)
	assertTimeRange(t, out, wantStart, wantEnd)
}

func TestResolveTimeRangeToolAcceptsMapInput(t *testing.T) {
	now, loc := fixedNow()
	tool := ResolveTimeRangeTool{
		Now:             func() time.Time { return now },
		DefaultLocation: loc,
	}
	out, err := tool.Execute(context.Background(), map[string]any{
		"kind":   "relative",
		"amount": float64(30),
		"unit":   "minute",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.(domain.TimeRange)
	wantStart := time.Date(2026, 7, 8, 10, 0, 0, 0, loc)
	assertTimeRange(t, got, wantStart, now)
}

func TestResolveTimeRangeToolCanonicalizesUnitAliases(t *testing.T) {
	now, loc := fixedNow()
	out := executeResolveTimeRange(t, ResolveTimeRangeInput{
		Kind:   "relative",
		Amount: 2,
		Unit:   "hours",
	}, now, loc)

	wantStart := time.Date(2026, 7, 8, 8, 30, 0, 0, loc)
	assertTimeRange(t, out, wantStart, now)
}

func TestResolveTimeRangeToolRejectsIrrelevantFields(t *testing.T) {
	now, loc := fixedNow()
	tool := ResolveTimeRangeTool{
		Now:             func() time.Time { return now },
		DefaultLocation: loc,
	}
	_, err := tool.Execute(context.Background(), ResolveTimeRangeInput{
		Kind:   "today",
		Amount: 1,
		Unit:   "hour",
	})
	if err == nil {
		t.Fatal("expected irrelevant field error")
	}
}

func TestResolveTimeRangeToolRejectsFutureTime(t *testing.T) {
	now, loc := fixedNow()
	tool := ResolveTimeRangeTool{
		Now:             func() time.Time { return now },
		DefaultLocation: loc,
	}
	_, err := tool.Execute(context.Background(), ResolveTimeRangeInput{
		Kind:      "since",
		StartText: "明天10点",
	})
	if err == nil {
		t.Fatal("expected future time error")
	}
}

func TestResolveTimeRangeFunctionToolSchema(t *testing.T) {
	def := ResolveTimeRangeFunctionTool()
	if def.Function.Name != ResolveTimeRangeToolName {
		t.Fatalf("unexpected function name: %s", def.Function.Name)
	}
	if got := def.Function.Parameters["additionalProperties"]; got != false {
		t.Fatalf("schema should disable additional properties: %+v", def.Function.Parameters)
	}
	required := def.Function.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "kind" {
		t.Fatalf("unexpected required fields: %+v", required)
	}
	properties := def.Function.Parameters["properties"].(map[string]any)
	unit := properties["unit"].(map[string]any)
	enum := unit["enum"].([]string)
	if len(enum) != 4 || enum[0] != "minute" || enum[1] != "hour" || enum[2] != "day" || enum[3] != "week" {
		t.Fatalf("schema should expose canonical units only: %+v", enum)
	}
}

func fixedNow() (time.Time, *time.Location) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	return time.Date(2026, 7, 8, 10, 30, 0, 0, loc), loc
}

func executeResolveTimeRange(t *testing.T, input ResolveTimeRangeInput, now time.Time, loc *time.Location) domain.TimeRange {
	t.Helper()
	tool := ResolveTimeRangeTool{
		Now:             func() time.Time { return now },
		DefaultLocation: loc,
	}
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return out.(domain.TimeRange)
}

func assertTimeRange(t *testing.T, got domain.TimeRange, wantStart, wantEnd time.Time) {
	t.Helper()
	if !got.Start.Equal(wantStart) {
		t.Fatalf("unexpected start: got=%s want=%s", got.Start, wantStart)
	}
	if !got.End.Equal(wantEnd) {
		t.Fatalf("unexpected end: got=%s want=%s", got.End, wantEnd)
	}
}
