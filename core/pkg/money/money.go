// Package money provides a deterministic integer type for monetary values.
// Amount is stored as nano-dollars (10^-9 USD) to avoid floating-point drift
// in cost accumulation and budget comparisons.
package money

import (
	"fmt"
	"strings"
)

// Amount is a monetary value in nano-dollars.
// All arithmetic is exact integer math. Range: ±$9.2 billion.
type Amount int64

const (
	NanoDollar  Amount = 1
	MicroDollar Amount = 1_000
	MilliDollar Amount = 1_000_000
	Dollar      Amount = 1_000_000_000
)

// FromDollars converts a float64 dollar value to Amount.
// Use only at system boundaries (config parsing, API input) — not on the hot path.
func FromDollars(d float64) Amount {
	return Amount(d * float64(Dollar))
}

// Dollars returns the value as float64. Use only for display and export, not arithmetic.
func (a Amount) Dollars() float64 {
	return float64(a) / float64(Dollar)
}

// String formats as a decimal dollar string, e.g. "1.25" or "0.0000025".
func (a Amount) String() string {
	neg := a < 0
	if neg {
		a = -a
	}
	d := int64(a) / int64(Dollar)
	f := int64(a) % int64(Dollar)
	s := strings.TrimRight(fmt.Sprintf("%d.%09d", d, f), "0")
	s = strings.TrimRight(s, ".")
	if neg {
		return "-" + s
	}
	return s
}
