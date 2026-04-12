package mappers

import (
	"time"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

// msSinceEpoch returns the current time in milliseconds since Unix epoch.
func msSinceEpoch() int64 {
	return time.Now().UnixMilli()
}

// msToTiming converts an int64 millisecond timestamp to timex.MS (or zero if nil).
// Deprecated: use msToTimingValue or ptrMSToTiming instead.
func msToTiming(ms *int64) timex.MS {
	if ms == nil {
		return timex.MS(0)
	}
	return timex.MS(*ms)
}

// msToTimingValue converts an int64 to timex.MS (value, not pointer).
func msToTimingValue(ms int64) timex.MS {
	return timex.MS(ms)
}

// ptrMSToTiming converts *int64 to *timex.MS (or nil if nil input).
func ptrMSToTiming(ms *int64) *timex.MS {
	if ms == nil {
		return nil
	}
	v := timex.MS(*ms)
	return &v
}

// ptrMSToTimingValue converts *int64 to timex.MS (value, not pointer, or zero if nil).
func ptrMSToTimingValue(ms *int64) timex.MS {
	if ms == nil {
		return timex.MS(0)
	}
	return timex.MS(*ms)
}

// ptrTimingToMS converts *timex.MS to *int64 (or nil if nil input).
func ptrTimingToMS(ms *timex.MS) *int64 {
	if ms == nil {
		return nil
	}
	v := int64(*ms)
	return &v
}

// timingToMS converts timex.MS to int64 (for non-pointer values).
func timingToMS(ms timex.MS) int64 {
	return int64(ms)
}

// ptrMS converts a timex.MS value to *int64 (or nil if zero).
func ptrMS(ms timex.MS) *int64 {
	if ms == 0 {
		return nil
	}
	v := int64(ms)
	return &v
}

// ptrMSFromTiming converts a timex.MS value to *int64 (or nil if zero).
// Alias for ptrMS for clarity.
func ptrMSFromTiming(ms timex.MS) *int64 {
	return ptrMS(ms)
}

// amountToUSD converts money.Amount to float64.
func amountToUSD(a money.Amount) float64 {
	return a.Dollars()
}

// ptrAmountToUSD converts *money.Amount to *float64 (nil-safe).
func ptrAmountToUSD(a *money.Amount) *float64 {
	if a == nil {
		return nil
	}
	v := a.Dollars()
	return &v
}

// usdToAmount converts float64 to money.Amount.
func usdToAmount(usd float64) money.Amount {
	return money.FromDollars(usd)
}

// ptrUSDToAmount converts *float64 to *money.Amount (nil-safe).
func ptrUSDToAmount(usd *float64) *money.Amount {
	if usd == nil {
		return nil
	}
	a := money.FromDollars(*usd)
	return &a
}
