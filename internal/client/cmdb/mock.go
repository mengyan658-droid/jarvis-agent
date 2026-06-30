package cmdb

import (
	"context"

	"jarvis-agent/internal/client"
)

type MockClient struct {
	Store    *client.MockStore
	Behavior client.MockBehavior
}

func NewMockClient(store *client.MockStore) *MockClient {
	return &MockClient{Store: store}
}

func (c *MockClient) QueryHostMetadata(ctx context.Context, hostID string) (map[string]string, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return nil, err
	}
	src := c.Store.CMDB[hostID]
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}
