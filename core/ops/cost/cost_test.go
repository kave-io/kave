package cost

import (
	"testing"

	"github.com/kave-io/kave/core/pkg/money"
)

func TestNewBudgetStatus(t *testing.T) {
	cap10 := money.FromDollars(10)
	cap5 := money.FromDollars(5)

	tests := []struct {
		name          string
		spent         money.Amount
		cap           *money.Amount
		period        string
		wantExceeded  bool
		wantRemaining *money.Amount
	}{
		{
			name:          "under budget",
			spent:         money.FromDollars(3),
			cap:           &cap10,
			period:        "run",
			wantExceeded:  false,
			wantRemaining: ptr(money.FromDollars(7)),
		},
		{
			name:          "exactly at cap",
			spent:         money.FromDollars(10),
			cap:           &cap10,
			period:        "daily",
			wantExceeded:  true,
			wantRemaining: ptr(money.Amount(0)),
		},
		{
			name:          "over budget",
			spent:         money.FromDollars(12),
			cap:           &cap10,
			period:        "monthly",
			wantExceeded:  true,
			wantRemaining: ptr(money.FromDollars(-2)),
		},
		{
			name:          "no cap (unlimited)",
			spent:         money.FromDollars(100),
			cap:           nil,
			period:        "run",
			wantExceeded:  false,
			wantRemaining: nil,
		},
		{
			name:          "zero spent with cap",
			spent:         0,
			cap:           &cap5,
			period:        "run",
			wantExceeded:  false,
			wantRemaining: &cap5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := NewBudgetStatus(tt.spent, tt.cap, tt.period)

			if bs.Spent != tt.spent {
				t.Errorf("Spent: got %v, want %v", bs.Spent, tt.spent)
			}
			if bs.Period != tt.period {
				t.Errorf("Period: got %q, want %q", bs.Period, tt.period)
			}
			if bs.Exceeded != tt.wantExceeded {
				t.Errorf("Exceeded: got %v, want %v", bs.Exceeded, tt.wantExceeded)
			}
			if tt.wantRemaining == nil {
				if bs.Remaining != nil {
					t.Errorf("Remaining: got %v, want nil", *bs.Remaining)
				}
			} else {
				if bs.Remaining == nil {
					t.Fatalf("Remaining: got nil, want %v", *tt.wantRemaining)
				}
				if *bs.Remaining != *tt.wantRemaining {
					t.Errorf("Remaining: got %v, want %v", *bs.Remaining, *tt.wantRemaining)
				}
			}
		})
	}
}

func TestNewBudgetStatus_ZeroCap(t *testing.T) {
	cap := money.Amount(0) // $0 cap
	spent := money.FromDollars(1)

	bs := NewBudgetStatus(spent, &cap, "run")

	if !bs.Exceeded {
		t.Errorf("Exceeded: got false, want true (spent >= cap)")
	}
	if bs.Remaining == nil {
		t.Fatal("Remaining: expected non-nil pointer")
	}
	// spent (1 dollar = 1_000_000_000 nanos) - cap (0) = remaining 1_000_000_000 (negative in absolute terms)
	expectedRemaining := money.FromDollars(0) - spent
	if *bs.Remaining != expectedRemaining {
		t.Errorf("Remaining: got %v, want %v", *bs.Remaining, expectedRemaining)
	}
}

func TestNewBudgetStatus_BothZero(t *testing.T) {
	var spent money.Amount // zero

	bs := NewBudgetStatus(spent, nil, "run")

	if bs.Exceeded {
		t.Errorf("Exceeded: got true, want false (no cap)")
	}
	if bs.Remaining != nil {
		t.Errorf("Remaining: got %v, want nil (no cap)", bs.Remaining)
	}
	if bs.Cap != nil {
		t.Errorf("Cap: got %v, want nil", bs.Cap)
	}
}

func ptr(a money.Amount) *money.Amount { return &a }
