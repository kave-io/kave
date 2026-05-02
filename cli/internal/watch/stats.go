package watch

import "strings"

func ApplyStats(stats Stats, ev Event) Stats {
	kind := strings.ToLower(ev.Kind)
	status := strings.ToLower(ev.Status)
	policy := strings.ToLower(ev.PolicyDecision)

	if strings.HasPrefix(kind, "run.") {
		switch {
		case strings.Contains(kind, "started") || strings.Contains(status, "active"):
			stats.ActiveRuns++
		case strings.Contains(kind, "completed") || strings.Contains(status, "completed"):
			stats.CompletedRuns++
		}
	}
	if strings.Contains(kind, "blocked") || strings.Contains(status, "blocked") || strings.Contains(policy, "deny") {
		stats.BlockedOrDenied++
	}
	if ev.Error != "" || strings.Contains(kind, "failed") || strings.Contains(status, "failed") || strings.Contains(kind, "error") {
		stats.Errors++
	}
	if ev.Cost > 0 {
		stats.TotalCost += ev.Cost
		if ev.Currency != "" {
			stats.Currency = ev.Currency
		}
	}
	return stats
}
