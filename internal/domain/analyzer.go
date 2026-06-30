package domain

type FaultAnalyzer struct{}

func NewFaultAnalyzer() *FaultAnalyzer {
	return &FaultAnalyzer{}
}

func (a *FaultAnalyzer) Analyze(e FaultEvidence) FaultAssessment {
	score := 0
	reasons := make([]string, 0)

	if !e.Host.Reachable {
		score += 50
		reasons = append(reasons, "host_unreachable")
	}
	if !e.Host.HealthCheckPassed {
		score += 30
		reasons = append(reasons, "health_check_failed")
	}

	critical := 0
	warning := 0
	for _, alarm := range e.Alarms {
		switch alarm.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		}
	}
	if critical > 0 {
		points := critical * 15
		if points > 45 {
			points = 45
		}
		score += points
		reasons = append(reasons, "critical_alarms")
	}
	if warning >= 3 {
		score += 10
		reasons = append(reasons, "multiple_warning_alarms")
	}

	if e.Metrics.CPUUsagePercent >= 95 {
		score += 15
		reasons = append(reasons, "cpu_usage_critical")
	} else if e.Metrics.CPUUsagePercent >= 85 {
		score += 8
		reasons = append(reasons, "cpu_usage_high")
	}
	if e.Metrics.MemoryUsagePercent >= 95 {
		score += 15
		reasons = append(reasons, "memory_usage_critical")
	}
	if e.Metrics.HighCPUDurationMinutes >= 10 {
		score += 10
		reasons = append(reasons, "cpu_high_load_sustained")
	}
	if len(e.RecentChanges) > 0 {
		score += 10
		reasons = append(reasons, "recent_changes")
	}
	if score > 100 {
		score = 100
	}

	return FaultAssessment{
		HostID:   e.Host.ID,
		Score:    score,
		Level:    levelForScore(score),
		IsFaulty: score >= 40,
		Reasons:  reasons,
		Evidence: e,
	}
}

func levelForScore(score int) FaultLevel {
	switch {
	case score <= 19:
		return FaultLevelNormal
	case score <= 39:
		return FaultLevelSuspicious
	case score <= 69:
		return FaultLevelDegraded
	default:
		return FaultLevelCritical
	}
}
