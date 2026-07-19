package requestcount

import (
	"context"
	"errors"
	"testing"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
)

func TestMockClientQueryErrorRequestCountsAggregatesByTimeModelIDCAndErrorCode(t *testing.T) {
	base := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	store := &client.MockStore{
		ErrorRequestCounts: []domain.ErrorRequestCountSample{
			{Timestamp: base.Add(10 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 5},
			{Timestamp: base.Add(45 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 7},
			{Timestamp: base.Add(70 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 3},
			{Timestamp: base.Add(80 * time.Minute), DeviceModel: "iphone-14", IDC: "shanghai-a", ErrorCode: "E500", Count: 11},
			{Timestamp: base.Add(90 * time.Minute), DeviceModel: "iphone-15", IDC: "beijing-a", ErrorCode: "E500", Count: 13},
			{Timestamp: base.Add(100 * time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E_TIMEOUT", Count: 17},
			{Timestamp: base.Add(-time.Minute), DeviceModel: "iphone-15", IDC: "shanghai-a", ErrorCode: "E500", Count: 99},
		},
	}
	got, err := NewMockClient(store).QueryErrorRequestCounts(context.Background(), client.ErrorRequestCountQuery{
		TimeRange:    domain.NewTimeRange(base, base.Add(2*time.Hour), base.Add(2*time.Hour), "UTC", "test", false),
		DeviceModels: []string{"iphone-15"},
		IDCs:         []string{"shanghai-a"},
		ErrorCode:    "E500",
		Aggregation:  domain.TimeAggregation{Value: 1, Unit: "h"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected result count: %+v", got)
	}
	if !got[0].BucketStart.Equal(base) || got[0].Count != 12 {
		t.Fatalf("unexpected first bucket: %+v", got[0])
	}
	if !got[1].BucketStart.Equal(base.Add(time.Hour)) || got[1].Count != 3 {
		t.Fatalf("unexpected second bucket: %+v", got[1])
	}
}

func TestMockClientQueryErrorRequestCountsSupportsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	c := NewMockClient(&client.MockStore{})
	c.Behavior.Delay = 20 * time.Millisecond

	_, err := c.QueryErrorRequestCounts(ctx, client.ErrorRequestCountQuery{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestMockClientQueryErrorRequestCountsSupportsForcedError(t *testing.T) {
	c := NewMockClient(&client.MockStore{})
	c.Behavior.ForceError = true

	_, err := c.QueryErrorRequestCounts(context.Background(), client.ErrorRequestCountQuery{})
	if !errors.Is(err, client.ErrForced) {
		t.Fatalf("expected forced error, got %v", err)
	}
}
