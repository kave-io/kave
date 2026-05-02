package watch

import (
	"fmt"
	"strings"
	"time"
)

func ShortID(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:8]
}

func TimeLabel(t time.Time) string {
	if t.IsZero() {
		return "--:--:--"
	}
	return t.Local().Format("15:04:05")
}

func CostLabel(cost float64, currency string) string {
	if cost <= 0 {
		return "-"
	}
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%.5f %s", cost, currency)
}

func CompactText(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
