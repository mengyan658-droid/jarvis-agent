package jarvis

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

func (c *MockClient) QueryHosts(ctx context.Context, query client.HostQuery) ([]domain.Host, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return nil, err
	}
	var hosts []domain.Host
	for _, host := range c.Store.Hosts {
		if query.Region != "" && host.Region != query.Region {
			continue
		}
		if query.Environment != "" && host.Environment != query.Environment {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (c *MockClient) GetHost(ctx context.Context, hostID string) (domain.Host, error) {
	if err := c.Behavior.Before(ctx); err != nil {
		return domain.Host{}, err
	}
	host, ok := c.Store.Hosts[hostID]
	if !ok {
		return domain.Host{}, errors.New("host not found")
	}
	return host, nil
}
