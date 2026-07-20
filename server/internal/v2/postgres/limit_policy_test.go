package postgres

import "testing"

func TestLimitPolicyOnlyChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields []string
		want   bool
	}{
		{name: "hard cap", fields: []string{"hard_cap"}, want: true},
		{name: "all policy fields", fields: []string{"enabled", "hard_cap", "soft_cap"}, want: true},
		{name: "no change"},
		{name: "metric", fields: []string{"metric"}},
		{name: "window", fields: []string{"window"}},
		{name: "policy and selector", fields: []string{"hard_cap", "tenant"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := limitPolicyOnlyChange(tt.fields); got != tt.want {
				t.Fatalf("limitPolicyOnlyChange(%v) = %v, want %v", tt.fields, got, tt.want)
			}
		})
	}
}
