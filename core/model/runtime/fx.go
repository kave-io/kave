package runtime

import "github.com/kave-io/kave/core/pkg/money"

// FXRateRecord is a single point-in-time FX rate from a named provider.
// Rate is decimal-encoded as a string (e.g. "0.9234") to avoid float drift.
type FXRateRecord struct {
	BaseCurrency  money.CurrencyCode
	QuoteCurrency money.CurrencyCode
	Rate          string
	Provider      string
	AsOfDate      string // ISO date (YYYY-MM-DD) the rate is quoted for
	FetchedAt     int64  // UnixMilli when the rate was retrieved
}

// FXCurrencyRecord is currency metadata cached from an external FX provider.
type FXCurrencyRecord struct {
	Code      money.CurrencyCode
	Name      string
	Symbol    string
	FetchedAt int64 // UnixMilli
}
