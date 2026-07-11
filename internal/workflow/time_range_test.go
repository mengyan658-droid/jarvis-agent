package workflow

import "testing"

func TestTimeRangeInputFromText(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantKind   string
		wantAmount int
		wantUnit   string
	}{
		{name: "default", text: "", wantKind: "default"},
		{name: "compact two days", text: "2d", wantKind: "relative", wantAmount: 2, wantUnit: "day"},
		{name: "compact one week", text: "1w", wantKind: "relative", wantAmount: 1, wantUnit: "week"},
		{name: "chinese recent two days", text: "最近2天", wantKind: "relative", wantAmount: 2, wantUnit: "day"},
		{name: "chinese near one week", text: "近一周", wantKind: "relative", wantAmount: 1, wantUnit: "week"},
		{name: "today", text: "today", wantKind: "today"},
		{name: "yesterday", text: "昨天", wantKind: "yesterday"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeRangeInputFromText(tt.text)
			if got.Kind != tt.wantKind || got.Amount != tt.wantAmount || got.Unit != tt.wantUnit {
				t.Fatalf("unexpected input: got=%+v want kind=%s amount=%d unit=%s", got, tt.wantKind, tt.wantAmount, tt.wantUnit)
			}
		})
	}
}

func TestTimeRangeInputFromParametersAbsoluteRange(t *testing.T) {
	got := timeRangeInputFromParameters(map[string]string{
		"start_text": "7月1号",
		"end_text":   "7月5号",
		"since":      "1h",
	})

	if got.Kind != "absolute_range" || got.StartText != "7月1号" || got.EndText != "7月5号" {
		t.Fatalf("unexpected input: %+v", got)
	}
}
