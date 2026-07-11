package agent

import "testing"

func TestExtractSince(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "recent five hours", text: "查询华东生产环境最近5小时的故障机", want: "5h"},
		{name: "recent thirty minutes", text: "查询最近30分钟的异常机器", want: "30m"},
		{name: "recent two days", text: "查询最近2天的故障机", want: "2d"},
		{name: "near one week", text: "查询近一周的故障机", want: "1w"},
		{name: "chinese five hours", text: "查询最近五小时的故障机", want: "5h"},
		{name: "today", text: "查询今天的故障机", want: "today"},
		{name: "yesterday", text: "查询昨天的故障机", want: "yesterday"},
		{name: "empty", text: "查询故障机", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSince(tt.text); got != tt.want {
				t.Fatalf("unexpected since: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestExtractAbsoluteTimeRange(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantStart string
		wantEnd   string
	}{
		{name: "chinese month day", text: "查询7月1号到7月5号的故障机", wantStart: "7月1号", wantEnd: "7月5号"},
		{name: "iso date", text: "查询2026-07-01 到 2026-07-05的故障机", wantStart: "2026-07-01", wantEnd: "2026-07-05"},
		{name: "slash date", text: "查询7/1至7/5的故障机", wantStart: "7/1", wantEnd: "7/5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := extractAbsoluteTimeRange(tt.text)
			if !ok {
				t.Fatal("expected absolute time range")
			}
			if start != tt.wantStart || end != tt.wantEnd {
				t.Fatalf("unexpected time range: start=%q end=%q", start, end)
			}
		})
	}
}
