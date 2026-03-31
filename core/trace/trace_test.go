package trace

import (
	"testing"

	"github.com/kave-io/kave/core/pkg/timex"
)

func TestSpan_ComputeDuration(t *testing.T) {
	tests := []struct {
		name      string
		startedAt timex.MS
		endedAt   timex.MS
		want      int64
	}{
		{"normal", timex.MS(1000), timex.MS(1500), 500},
		{"zero start", timex.MS(0), timex.MS(1500), 0},
		{"zero end", timex.MS(1000), timex.MS(0), 0},
		{"both zero", timex.MS(0), timex.MS(0), 0},
		{"same time", timex.MS(1000), timex.MS(1000), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Span{StartedAt: tt.startedAt, EndedAt: tt.endedAt}
			got := s.ComputeDuration()
			if got != tt.want {
				t.Errorf("ComputeDuration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSourceConstants(t *testing.T) {
	tests := []struct {
		got  string
		want string
	}{
		{SourceIntercept, "intercept"},
		{SourceReport, "report"},
		{SourceOTELImport, "otel_import"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("source constant %q != %q", tt.got, tt.want)
		}
	}
}
