package monitor

import (
	"context"
	"errors"

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

func (c *MockClient) QueryHostMetrics(ctx context.Context, hostID string) (domain.HostMetrics, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return domain.HostMetrics{}, err
	}
	metrics, ok := c.Store.Metrics[hostID]
	if !ok {
		return domain.HostMetrics{}, errors.New("metrics not found")
	}
	return metrics, nil
}

func (c *MockClient) QueryActiveAlarms(ctx context.Context, hostID string) ([]domain.Alarm, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return nil, err
	}
	return append([]domain.Alarm(nil), c.Store.Alarms[hostID]...), nil
}
