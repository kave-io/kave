package watch

import (
	"testing"
	"time"
)

func TestShortID(t *testing.T) {
	if got := ShortID("1234567890"); got != "12345678" {
		t.Fatalf("ShortID got %q", got)
	}
}

func TestCostLabel(t *testing.T) {
	if got := CostLabel(0.123456, "USD"); got != "0.12346 USD" {
		t.Fatalf("CostLabel got %q", got)
	}
}

func TestTimeLabel(t *testing.T) {
	got := TimeLabel(time.Unix(0, 0))
	if got == "--:--:--" {
		t.Fatalf("TimeLabel should format non-zero time")
	}
}
