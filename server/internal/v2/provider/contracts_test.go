package provider

import (
	"math"
	"testing"
)

func TestCalculateUsageCostDetailedDimensions(t *testing.T) {
	t.Parallel()
	price := Price{
		InputNanosPerMillionTokens:      2_000_000,
		OutputNanosPerMillionTokens:     8_000_000,
		CacheReadNanosPerMillionTokens:  500_000,
		CacheWriteNanosPerMillionTokens: 3_000_000,
		ReasoningNanosPerMillionTokens:  12_000_000,
	}
	usage := Usage{
		InputTokens: 100, CacheReadTokens: 40, CacheWriteTokens: 10,
		OutputTokens: 20, ReasoningTokens: 5,
	}
	cost, ok := CalculateUsageCost(price, usage)
	if !ok {
		t.Fatal("CalculateUsageCost rejected valid usage")
	}
	// 60*2 + 40*.5 + 10*3 + 15*8 + 5*12 = 350 nano-USD.
	if cost != 350 {
		t.Fatalf("cost = %d, want 350", cost)
	}
}

func TestCalculateUsageCostCompatibilityDefaults(t *testing.T) {
	t.Parallel()
	price := Price{InputNanosPerMillionTokens: 2_000_000, OutputNanosPerMillionTokens: 4_000_000}
	usage := Usage{InputTokens: 100, CacheReadTokens: 25, OutputTokens: 20, ReasoningTokens: 5}
	cost, ok := CalculateUsageCost(price, usage)
	if !ok || cost != 280 {
		t.Fatalf("compatibility cost = %d, %v; want 280, true", cost, ok)
	}
	basic, ok := CalculateUsageCost(price, Usage{InputTokens: 100, OutputTokens: 20})
	if !ok || basic != 280 {
		t.Fatalf("basic cost = %d, %v; want 280, true", basic, ok)
	}
}

func TestCalculateMaximumCostIsConservative(t *testing.T) {
	t.Parallel()
	price := Price{
		InputNanosPerMillionTokens: 2_000_000, OutputNanosPerMillionTokens: 4_000_000,
		CacheReadNanosPerMillionTokens: 1_000_000, CacheWriteNanosPerMillionTokens: 3_000_000,
		ReasoningNanosPerMillionTokens: 8_000_000,
	}
	reserved, ok := CalculateMaximumCost(price, 100, 20)
	if !ok || reserved != 660 {
		t.Fatalf("maximum cost = %d, %v; want 660, true", reserved, ok)
	}
	actual, ok := CalculateUsageCost(price, Usage{
		InputTokens: 100, CacheReadTokens: 75, CacheWriteTokens: 100,
		OutputTokens: 20, ReasoningTokens: 20,
	})
	if !ok || actual > reserved {
		t.Fatalf("actual cost = %d, %v exceeds reservation %d", actual, ok, reserved)
	}
}

func TestCalculateUsageCostRejectsImpossibleUsageAndOverflow(t *testing.T) {
	t.Parallel()
	price := Price{InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 1}
	for _, usage := range []Usage{
		{InputTokens: -1},
		{InputTokens: 1, CacheReadTokens: 2},
		{InputTokens: 1, CacheWriteTokens: 2},
		{OutputTokens: 1, ReasoningTokens: 2},
	} {
		if _, ok := CalculateUsageCost(price, usage); ok {
			t.Fatalf("CalculateUsageCost accepted %+v", usage)
		}
	}
	if _, ok := CalculateMaximumCost(Price{InputNanosPerMillionTokens: math.MaxInt64}, math.MaxInt64, 0); ok {
		t.Fatal("CalculateMaximumCost accepted overflow")
	}
}
