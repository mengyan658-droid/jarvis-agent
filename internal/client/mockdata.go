package client

import (
	"time"

	"jarvis-agent/internal/domain"
)

type MockStore struct {
	Hosts   map[string]domain.Host
	Metrics map[string]domain.HostMetrics
	Alarms  map[string][]domain.Alarm
	Changes map[string][]domain.ChangeRecord
	CMDB    map[string]map[string]string
}

func NewMockStore() *MockStore {
	now := time.Now()
	return &MockStore{
		Hosts: map[string]domain.Host{
			"host-001": {ID: "host-001", Name: "pay-api-01", Region: "east-china", Environment: "production", IP: "10.0.1.11", Reachable: true, HealthCheckPassed: true},
			"host-002": {ID: "host-002", Name: "pay-api-02", Region: "east-china", Environment: "production", IP: "10.0.1.12", Reachable: true, HealthCheckPassed: true},
			"host-003": {ID: "host-003", Name: "order-api-01", Region: "north-china", Environment: "staging", IP: "10.1.1.11", Reachable: false, HealthCheckPassed: false},
			"host-004": {ID: "host-004", Name: "search-01", Region: "east-china", Environment: "production", IP: "10.0.2.21", Reachable: true, HealthCheckPassed: true},
			"host-005": {ID: "host-005", Name: "asset-api-01", Region: "south-china", Environment: "production", IP: "10.2.1.11", Reachable: true, HealthCheckPassed: true},
		},
		Metrics: map[string]domain.HostMetrics{
			"host-001": {HostID: "host-001", CPUUsagePercent: 96, MemoryUsagePercent: 76, HighCPUDurationMinutes: 12, CollectedAt: now},
			"host-002": {HostID: "host-002", CPUUsagePercent: 22, MemoryUsagePercent: 41, CollectedAt: now},
			"host-003": {HostID: "host-003", CPUUsagePercent: 10, MemoryUsagePercent: 20, CollectedAt: now},
			"host-004": {HostID: "host-004", CPUUsagePercent: 87, MemoryUsagePercent: 72, HighCPUDurationMinutes: 5, CollectedAt: now},
			"host-005": {HostID: "host-005", CPUUsagePercent: 45, MemoryUsagePercent: 49, CollectedAt: now},
		},
		Alarms: map[string][]domain.Alarm{
			"host-001": {{ID: "alm-001", HostID: "host-001", Severity: "critical", Message: "CPU saturation", StartedAt: now.Add(-20 * time.Minute)}},
			"host-004": {
				{ID: "alm-004-a", HostID: "host-004", Severity: "warning", Message: "CPU high", StartedAt: now.Add(-18 * time.Minute)},
				{ID: "alm-004-b", HostID: "host-004", Severity: "warning", Message: "request latency high", StartedAt: now.Add(-16 * time.Minute)},
				{ID: "alm-004-c", HostID: "host-004", Severity: "warning", Message: "queue backlog", StartedAt: now.Add(-12 * time.Minute)},
			},
		},
		Changes: map[string][]domain.ChangeRecord{
			"host-001": {{ID: "chg-001", HostID: "host-001", Type: "deploy", Description: "payment service deployment", Risk: domain.RiskLevelHigh, CreatedAt: now.Add(-30 * time.Minute)}},
			"host-005": {{ID: "chg-005", HostID: "host-005", Type: "config", Description: "normal config refresh", Risk: domain.RiskLevelLow, CreatedAt: now.Add(-40 * time.Minute)}},
		},
		CMDB: map[string]map[string]string{
			"host-001": {"owner": "payment", "service": "payment-api"},
			"host-002": {"owner": "payment", "service": "payment-api"},
			"host-003": {"owner": "order", "service": "order-api"},
			"host-004": {"owner": "search", "service": "search-api"},
			"host-005": {"owner": "asset", "service": "asset-api"},
		},
	}
}
