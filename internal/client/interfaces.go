package client

import (
	"context"
	"time"

	"jarvis-agent/internal/domain"
)

type HostQuery struct {
	Region      string
	Environment string
}

type JarvisClient interface {
	QueryHosts(ctx context.Context, query HostQuery) ([]domain.Host, error)
	GetHost(ctx context.Context, hostID string) (domain.Host, error)
}

type MonitorClient interface {
	QueryHostMetrics(ctx context.Context, hostID string) (domain.HostMetrics, error)
	QueryActiveAlarms(ctx context.Context, hostID string) ([]domain.Alarm, error)
}

type ChangeClient interface {
	QueryRecentChanges(ctx context.Context, hostID string, since time.Time) ([]domain.ChangeRecord, error)
}

type CMDBClient interface {
	QueryHostMetadata(ctx context.Context, hostID string) (map[string]string, error)
}

type Intent struct {
	Name       string            `json:"name"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type LLMClient interface {
	ParseIntent(ctx context.Context, message string) (Intent, error)
	GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error)
	GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error)
}
