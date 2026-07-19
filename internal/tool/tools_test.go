package tool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/client/change"
	"jarvis-agent/internal/client/jarvis"
	"jarvis-agent/internal/client/requestcount"
	"jarvis-agent/internal/domain"
)

func TestRegistryRecordsToolCalls(t *testing.T) {
	store := client.NewMockStore()
	registry := NewRegistry(QueryHostsTool{Client: jarvis.NewMockClient(store)})
	recorder := &Recorder{}
	out, err := registry.Execute(context.Background(), QueryHostsToolName, client.HostQuery{Region: "east-china"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.([]domain.Host)) == 0 {
		t.Fatal("unexpected empty output")
	}
	if len(recorder.Calls()) != 1 || recorder.Calls()[0].Name != QueryHostsToolName {
		t.Fatalf("unexpected calls: %+v", recorder.Calls())
	}
}

func TestRegistryWritesQueryChangesTimeTestLog(t *testing.T) {
	base := time.Date(2026, 7, 8, 10, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	store := &client.MockStore{
		Changes: map[string][]domain.ChangeRecord{"host-001": {}},
	}
	logPath := t.TempDir() + "/time-test.log"
	registry := NewRegistry(QueryChangesTool{Client: change.NewMockClient(store)}).WithTimeTestLogPath(logPath)

	_, err := registry.Execute(ContextWithRequestID(context.Background(), "req-time-test"), QueryChangesToolName, QueryChangesInput{
		HostID:    "host-001",
		TimeRange: domain.NewTimeRange(base.Add(-time.Hour), base, base, "Asia/Shanghai", "relative", false),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{`"request_id":"req-time-test"`, `"tool":"query_changes"`, `"duration_sec":3600`, `"host_id":"host-001"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("time test log missing %s: %s", want, got)
		}
	}
}

func TestQueryChangesToolUsesResolvedTimeRange(t *testing.T) {
	base := time.Date(2026, 7, 8, 10, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	store := &client.MockStore{
		Changes: map[string][]domain.ChangeRecord{
			"host-001": {
				{ID: "chg-in-range", HostID: "host-001", CreatedAt: base.Add(-36 * time.Hour)},
				{ID: "chg-too-old", HostID: "host-001", CreatedAt: base.Add(-8 * 24 * time.Hour)},
				{ID: "chg-at-end", HostID: "host-001", CreatedAt: base},
			},
		},
	}
	resolved := domain.NewTimeRange(base.Add(-48*time.Hour), base, base, "Asia/Shanghai", "relative", false)

	out, err := QueryChangesTool{Client: change.NewMockClient(store)}.Execute(context.Background(), QueryChangesInput{
		HostID:    "host-001",
		TimeRange: resolved,
	})
	if err != nil {
		t.Fatal(err)
	}
	changes := out.([]domain.ChangeRecord)
	if len(changes) != 1 || changes[0].ID != "chg-in-range" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}

func TestQueryErrorRequestCountsTool(t *testing.T) {
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	store := &client.MockStore{
		ErrorRequestCounts: []domain.ErrorRequestCountSample{
			{Timestamp: base.Add(10 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 5},
			{Timestamp: base.Add(20 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 6},
		},
	}
	out, err := QueryErrorRequestCountsTool{Client: requestcount.NewMockClient(store)}.Execute(context.Background(), QueryErrorRequestCountsInput{
		TimeRange:    domain.NewTimeRange(base, base.Add(time.Hour), base.Add(time.Hour), "UTC", "test", false),
		DeviceModels: []string{"iphone-15"},
		IDCs:         []string{"shanghai-a"},
		ErrorCode:    "E500",
		Aggregation:  domain.TimeAggregation{Value: 1, Unit: "h"},
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := out.([]domain.ErrorRequestCount)
	if len(counts) != 1 || counts[0].Count != 11 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestQueryErrorRequestCountsToolValidatesAggregation(t *testing.T) {
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	_, err := QueryErrorRequestCountsTool{Client: requestcount.NewMockClient(&client.MockStore{})}.Execute(context.Background(), QueryErrorRequestCountsInput{
		TimeRange:   domain.NewTimeRange(base, base.Add(time.Hour), base.Add(time.Hour), "UTC", "test", false),
		Aggregation: domain.TimeAggregation{Value: 0, Unit: "h"},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
