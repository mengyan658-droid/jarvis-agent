package domain

import "testing"

func TestFaultAnalyzerCriticalHost(t *testing.T) {
	a := NewFaultAnalyzer()
	got := a.Analyze(FaultEvidence{
		Host: Host{ID: "host-003", Reachable: false, HealthCheckPassed: false},
	})
	if got.Score != 80 || got.Level != FaultLevelCritical || !got.IsFaulty {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestFaultAnalyzerCapsScore(t *testing.T) {
	a := NewFaultAnalyzer()
	got := a.Analyze(FaultEvidence{
		Host: Host{ID: "host-001", Reachable: false, HealthCheckPassed: false},
		Metrics: HostMetrics{
			CPUUsagePercent:        99,
			MemoryUsagePercent:     99,
			HighCPUDurationMinutes: 20,
		},
		Alarms: []Alarm{
			{Severity: "critical"}, {Severity: "critical"}, {Severity: "critical"}, {Severity: "critical"},
		},
		RecentChanges: []ChangeRecord{{ID: "chg-1"}},
	})
	if got.Score != 100 {
		t.Fatalf("score should be capped at 100, got %d", got.Score)
	}
}

func TestFaultAnalyzerSuspiciousNotFaulty(t *testing.T) {
	a := NewFaultAnalyzer()
	got := a.Analyze(FaultEvidence{
		Host:    Host{ID: "host-004", Reachable: true, HealthCheckPassed: true},
		Metrics: HostMetrics{CPUUsagePercent: 87},
		Alarms:  []Alarm{{Severity: "warning"}, {Severity: "warning"}, {Severity: "warning"}},
	})
	if got.Score != 18 || got.Level != FaultLevelNormal || got.IsFaulty {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}
