package domain

import "time"

type FaultLevel string

const (
	FaultLevelNormal     FaultLevel = "normal"
	FaultLevelSuspicious FaultLevel = "suspicious"
	FaultLevelDegraded   FaultLevel = "degraded"
	FaultLevelCritical   FaultLevel = "critical"
)

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type Host struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Region            string            `json:"region"`
	Environment       string            `json:"environment"`
	IP                string            `json:"ip"`
	Reachable         bool              `json:"reachable"`
	HealthCheckPassed bool              `json:"health_check_passed"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type Alarm struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
}

type ChangeRecord struct {
	ID          string    `json:"id"`
	HostID      string    `json:"host_id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Risk        RiskLevel `json:"risk"`
	CreatedAt   time.Time `json:"created_at"`
}

type HostMetrics struct {
	HostID                 string    `json:"host_id"`
	CPUUsagePercent        float64   `json:"cpu_usage_percent"`
	MemoryUsagePercent     float64   `json:"memory_usage_percent"`
	HighCPUDurationMinutes int       `json:"high_cpu_duration_minutes"`
	CollectedAt            time.Time `json:"collected_at"`
}

type FaultEvidence struct {
	Host          Host              `json:"host"`
	Metrics       HostMetrics       `json:"metrics"`
	Alarms        []Alarm           `json:"alarms"`
	RecentChanges []ChangeRecord    `json:"recent_changes"`
	CMDB          map[string]string `json:"cmdb,omitempty"`
}

type FaultAssessment struct {
	HostID   string        `json:"host_id"`
	Score    int           `json:"score"`
	Level    FaultLevel    `json:"level"`
	IsFaulty bool          `json:"is_faulty"`
	Reasons  []string      `json:"reasons"`
	Evidence FaultEvidence `json:"evidence"`
}
