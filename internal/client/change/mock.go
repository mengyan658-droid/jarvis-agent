package change

import (
	"context"

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

func (c *MockClient) QueryRecentChanges(ctx context.Context, hostID string, timeRange domain.TimeRange) ([]domain.ChangeRecord, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return nil, err
	}
	records := c.Store.Changes[hostID]
	out := make([]domain.ChangeRecord, 0, len(records))
	for _, record := range records {
		if !record.CreatedAt.Before(timeRange.Start) && record.CreatedAt.Before(timeRange.End) {
			out = append(out, record)
		}
	}
	return out, nil
}
