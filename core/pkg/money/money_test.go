package money

import (
	"math"
	"testing"
)

func TestUnitConstants(t *testing.T) {
	cases := []struct {
		name  string
		unit  Amount
		value int64
	}{
		{"NanoDollar is 1", NanoDollar, 1},
		{"MicroDollar is 1000", MicroDollar, 1_000},
		{"MilliDollar is 1000000", MilliDollar, 1_000_000},
		{"Dollar is 1000000000", Dollar, 1_000_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if int64(c.unit) != c.value {
				t.Errorf("got %d, want %d", c.unit, c.value)
			}
		})
	}
}

func TestUnitRelationships(t *testing.T) {
	if MicroDollar != 1_000*NanoDollar {
		t.Error("MicroDollar != 1000 * NanoDollar")
	}
	if MilliDollar != 1_000*MicroDollar {
		t.Error("MilliDollar != 1000 * MicroDollar")
	}
	if Dollar != 1_000*MilliDollar {
		t.Error("Dollar != 1000 * MilliDollar")
	}
	if Dollar != 1_000_000_000*NanoDollar {
		t.Error("Dollar != 10^9 * NanoDollar")
	}
}

func TestFromDollars(t *testing.T) {
	cases := []struct {
		name    string
		dollars float64
		want    Amount
	}{
		{"zero", 0.0, 0},
		{"one dollar", 1.0, Dollar},
		{"ten dollars", 10.0, 10 * Dollar},
		{"one thousand dollars", 1000.0, 1000 * Dollar},
		{"half dollar", 0.50, 500 * MilliDollar},
		{"one cent", 0.01, 10 * MilliDollar},
		{"sub-cent GPT-4o input $2.50/1M", 2.50, 2*Dollar + 500*MilliDollar},
		{"sub-cent value $0.001", 0.001, MilliDollar},
		{"negative one dollar", -1.0, -Dollar},
		{"negative half dollar", -0.50, -500 * MilliDollar},
		{"very small $0.0000025", 0.0000025, 2500 * NanoDollar},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromDollars(c.dollars)
			if got != c.want {
				t.Errorf("FromDollars(%v) = %d, want %d", c.dollars, got, c.want)
			}
		})
	}
}

func TestDollarsRoundtrip(t *testing.T) {
	cases := []struct {
		name    string
		dollars float64
	}{
		{"zero", 0.0},
		{"one", 1.0},
		{"ten", 10.0},
		{"half", 0.5},
		{"one cent", 0.01},
		{"negative", -1.0},
		{"large", 1000.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromDollars(c.dollars).Dollars()
			if math.Abs(got-c.dollars) > 1e-9 {
				t.Errorf("roundtrip: FromDollars(%v).Dollars() = %v", c.dollars, got)
			}
		})
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		name   string
		amount Amount
		want   string
	}{
		{"zero", 0, "0"},
		{"one dollar", Dollar, "1"},
		{"one dollar fifty", Dollar + 500*MilliDollar, "1.5"},
		{"trailing zeros stripped $1.00025", Dollar + 250*MicroDollar, "1.00025"},
		{"sub-dollar $0.50", 500 * MilliDollar, "0.5"},
		{"nano precision 2500 nano", 2500 * NanoDollar, "0.0000025"},
		{"one nano-dollar", NanoDollar, "0.000000001"},
		{"negative one dollar", -Dollar, "-1"},
		{"negative fractional", -(Dollar + 500*MilliDollar), "-1.5"},
		{"large $1000", 1000 * Dollar, "1000"},
		{"large $9999.99", 9999*Dollar + 990*MilliDollar, "9999.99"},
		{"one micro-dollar", MicroDollar, "0.000001"},
		{"one milli-dollar", MilliDollar, "0.001"},
		{"negative nano", -NanoDollar, "-0.000000001"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.amount.String(); got != c.want {
				t.Errorf("Amount(%d).String() = %q, want %q", c.amount, got, c.want)
			}
		})
	}
}

func TestTokenAccumulation(t *testing.T) {
	cases := []struct {
		name        string
		priceNano   Amount
		tokens      int
		wantDollars Amount
	}{
		{
			"GPT-4o input: $2.50/1M tokens",
			2500 * NanoDollar,
			1_000_000,
			2*Dollar + 500*MilliDollar,
		},
		{
			"GPT-4o output: $10.00/1M tokens",
			10_000 * NanoDollar,
			1_000_000,
			10 * Dollar,
		},
		{
			"cheap model: $0.10/1M tokens",
			100 * NanoDollar,
			1_000_000,
			100 * MilliDollar,
		},
		{
			"single token at 2500 nano",
			2500 * NanoDollar,
			1,
			2500 * NanoDollar,
		},
		{
			"zero tokens",
			2500 * NanoDollar,
			0,
			0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Amount(int64(c.priceNano) * int64(c.tokens))
			if got != c.wantDollars {
				t.Errorf("got %s (%d nano), want %s (%d nano)", got, got, c.wantDollars, c.wantDollars)
			}
		})
	}
}

func TestBudgetComparison(t *testing.T) {
	cap := 10 * Dollar
	var spent Amount
	for range 10 {
		spent += Dollar
	}
	if spent != cap {
		t.Errorf("10 x $1 != $10: got %s, want %s", spent, cap)
	}
}

func TestBudgetComparisonSubCent(t *testing.T) {
	// Accumulate 100 x $0.01 and compare to $1.00
	cap := Dollar
	var spent Amount
	for range 100 {
		spent += 10 * MilliDollar // $0.01
	}
	if spent != cap {
		t.Errorf("100 x $0.01 != $1.00: got %s, want %s", spent, cap)
	}
}

func TestAdditionSubtraction(t *testing.T) {
	a := 3 * Dollar
	b := Dollar + 500*MilliDollar

	sum := a + b
	if sum != 4*Dollar+500*MilliDollar {
		t.Errorf("$3 + $1.50 = %s, want 4.5", sum)
	}

	diff := a - b
	if diff != Dollar+500*MilliDollar {
		t.Errorf("$3 - $1.50 = %s, want 1.5", diff)
	}

	// Subtraction resulting in negative
	neg := b - a
	if neg != -(Dollar + 500*MilliDollar) {
		t.Errorf("$1.50 - $3 = %s, want -1.5", neg)
	}
}

func TestMultiplication(t *testing.T) {
	price := 2 * Dollar
	qty := int64(5)
	got := Amount(int64(price) * qty)
	if got != 10*Dollar {
		t.Errorf("$2 * 5 = %s, want $10", got)
	}
}

func TestAmountAsMapKey(t *testing.T) {
	m := make(map[Amount]string)
	m[Dollar] = "one"
	m[2*Dollar] = "two"

	if m[Dollar] != "one" {
		t.Error("Amount as map key: lookup for Dollar failed")
	}
	if m[2*Dollar] != "two" {
		t.Error("Amount as map key: lookup for 2*Dollar failed")
	}

	// Same value, different construction
	m[1_000_000_000] = "also one"
	if m[Dollar] != "also one" {
		t.Error("Amount equality: Dollar != Amount(1_000_000_000)")
	}
}

func TestOverflowBoundary(t *testing.T) {
	// int64 max = 9,223,372,036,854,775,807 nano-dollars
	// That's $9,223,372,036.854775807 — about $9.2 billion
	maxAmount := Amount(math.MaxInt64)
	maxDollars := maxAmount.Dollars()

	// Verify it's approximately $9.2 billion
	if maxDollars < 9_000_000_000 || maxDollars > 10_000_000_000 {
		t.Errorf("int64 max in dollars = %f, expected ~$9.2B", maxDollars)
	}

	// Verify exact dollar boundary
	nineBillion := Amount(9_000_000_000) * Dollar
	if nineBillion.Dollars() != 9_000_000_000.0 {
		t.Errorf("$9B representation: got %f", nineBillion.Dollars())
	}

	// Verify String still works at high values
	s := nineBillion.String()
	if s != "9000000000" {
		t.Errorf("$9B String() = %q, want \"9000000000\"", s)
	}
}

func TestNegativeFromDollarsString(t *testing.T) {
	a := FromDollars(-2.50)
	if a.String() != "-2.5" {
		t.Errorf("FromDollars(-2.50).String() = %q, want \"-2.5\"", a.String())
	}
}

func TestAccumulationNoDrift(t *testing.T) {
	// Simulate a real billing scenario: 10M tokens across 100 requests of 100K each
	pricePerToken := Amount(2500) // 2500 nano per token
	var total Amount
	for range 100 {
		batch := Amount(int64(pricePerToken) * 100_000)
		total += batch
	}
	want := Amount(int64(pricePerToken) * 10_000_000)
	if total != want {
		t.Errorf("accumulated drift: got %d, want %d (diff=%d)", total, want, total-want)
	}
}
