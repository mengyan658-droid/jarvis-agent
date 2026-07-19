package requestcount

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
)

type MockClient struct {
	Store    *client.MockStore
	Behavior client.MockBehavior
}

func NewMockClient(store *client.MockStore) *MockClient {
	return &MockClient{Store: store}
}

func (c *MockClient) QueryErrorRequestCounts(ctx context.Context, query client.ErrorRequestCountQuery) ([]domain.ErrorRequestCount, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return nil, err
	}
	if c.Store == nil {
		return nil, fmt.Errorf("mock store is required")
	}
	if query.TimeRange.Start.IsZero() || query.TimeRange.End.IsZero() {
		return nil, fmt.Errorf("time_range start and end are required")
	}
	if !query.TimeRange.Start.Before(query.TimeRange.End) {
		return nil, fmt.Errorf("time_range start must be before end")
	}
	bucketDuration, err := query.Aggregation.Duration()
	if err != nil {
		return nil, err
	}

	models := stringSet(query.DeviceModels)
	idcs := stringSet(query.IDCs)
	errorCode := strings.TrimSpace(query.ErrorCode)
	buckets := map[string]*domain.ErrorRequestCount{}

	for _, sample := range c.Store.ErrorRequestCounts {
		if sample.Count <= 0 {
			continue
		}
		if sample.Timestamp.Before(query.TimeRange.Start) || !sample.Timestamp.Before(query.TimeRange.End) {
			continue
		}
		if len(models) > 0 && !models[sample.DeviceModel] {
			continue
		}
		if len(idcs) > 0 && !idcs[sample.IDC] {
			continue
		}
		if errorCode != "" && sample.ErrorCode != errorCode {
			continue
		}
		bucketStart := bucketStartFor(sample.Timestamp, query.TimeRange.Start, bucketDuration)
		bucketEnd := bucketStart.Add(bucketDuration)
		if bucketEnd.After(query.TimeRange.End) {
			bucketEnd = query.TimeRange.End
		}
		key := bucketKey(bucketStart, sample.DeviceModel, sample.IDC, sample.ErrorCode)
		record, ok := buckets[key]
		if !ok {
			record = &domain.ErrorRequestCount{
				BucketStart: bucketStart,
				BucketEnd:   bucketEnd,
				DeviceModel: sample.DeviceModel,
				IDC:         sample.IDC,
				ErrorCode:   sample.ErrorCode,
			}
			buckets[key] = record
		}
		record.Count += sample.Count
	}

	out := make([]domain.ErrorRequestCount, 0, len(buckets))
	for _, record := range buckets {
		out = append(out, *record)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].BucketStart.Equal(out[j].BucketStart) {
			return out[i].BucketStart.Before(out[j].BucketStart)
		}
		if out[i].DeviceModel != out[j].DeviceModel {
			return out[i].DeviceModel < out[j].DeviceModel
		}
		if out[i].IDC != out[j].IDC {
			return out[i].IDC < out[j].IDC
		}
		return out[i].ErrorCode < out[j].ErrorCode
	})
	return out, nil
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func bucketStartFor(ts, rangeStart time.Time, bucketDuration time.Duration) time.Time {
	offset := ts.Sub(rangeStart)
	if offset < 0 {
		return rangeStart
	}
	index := int64(offset / bucketDuration)
	return rangeStart.Add(time.Duration(index) * bucketDuration)
}

func bucketKey(bucketStart time.Time, model, idc, errorCode string) string {
	return fmt.Sprintf("%d|%s|%s|%s", bucketStart.UnixNano(), model, idc, errorCode)
}
