package fx

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

// Service manages foreign exchange rates with Frankfurter sync.
// It allows concurrent reads via cached snapshots, and refreshes via Frankfurter.
// Refresh is triggered on startup, periodically every 60s, and on-demand via RPC.
//
// B2 Exception: FX uses a single 60s refresh ticker as an approved observability
// exception (declared in linter allowlist). The service observes Frankfurter rates,
// never initiates agent actions.
type Service struct {
	app      store.AppStore
	client   *http.Client
	irt_per_usd int64 // operator-set Toman/USD rate in micro (rate × 1e6)

	// Background refresh ticker
	mu         sync.RWMutex
	rates      map[string]runtimemodel.FXRateRecord // key: "base/quote"
	currencies map[money.CurrencyCode]runtimemodel.FXCurrencyRecord
	lastRefresh int64 // UnixMilli
	refreshErr  error
	ticker      *time.Ticker
	tickerDone  chan struct{}
}

// NewService creates a new FX service with the given Frankfurter client and IRT/USD rate.
// irt_per_usd should be the operator-configured rate in micro (rate × 1e6).
// E.g., 60_000.5 IRT/USD → 60_000_500_000.
func NewService(app store.AppStore, irt_per_usd int64) *Service {
	if irt_per_usd <= 0 {
		irt_per_usd = 60_000_000_000 // default: 60_000 IRT/USD
	}
	return &Service{
		app:         app,
		client:      &http.Client{Timeout: 15 * time.Second},
		irt_per_usd: irt_per_usd,
		rates:       make(map[string]runtimemodel.FXRateRecord),
		currencies:  make(map[money.CurrencyCode]runtimemodel.FXCurrencyRecord),
		tickerDone:  make(chan struct{}),
	}
}

// Load reads rates from storage into the in-memory cache.
func (s *Service) Load(ctx context.Context) error {
	rates, err := s.app.ListFXRates(ctx)
	if err != nil {
		return err
	}
	currencies, err := s.app.ListFXCurrencies(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rates = make(map[string]runtimemodel.FXRateRecord, len(rates))
	for _, rate := range rates {
		s.rates[rateKey(rate.BaseCurrency, rate.QuoteCurrency)] = rate
	}
	s.currencies = make(map[money.CurrencyCode]runtimemodel.FXCurrencyRecord, len(currencies))
	for _, item := range currencies {
		s.currencies[item.Code] = item
	}
	return nil
}

// StartRefresh begins the background 60s refresh ticker.
// The first refresh is non-blocking; failures log at warn level.
// If ctx is cancelled, the ticker stops.
func (s *Service) StartRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		s.mu.Lock()
		s.ticker = ticker
		s.mu.Unlock()

		for {
			select {
			case <-ticker.C:
				if err := s.Refresh(context.Background()); err != nil {
					log.Printf("warn: fx refresh tick failed: %v", err)
					s.mu.Lock()
					s.refreshErr = err
					s.mu.Unlock()
				} else {
					if err := s.Load(context.Background()); err != nil {
						log.Printf("warn: fx load after refresh failed: %v", err)
					}
				}
			case <-ctx.Done():
				ticker.Stop()
				close(s.tickerDone)
				return
			}
		}
	}()
}

// Refresh fetches the latest rates from Frankfurter and upserts them into storage.
// On transient failures (network, 5xx), the operation logs and returns the error;
// the cache remains valid from the last successful refresh.
func (s *Service) Refresh(ctx context.Context) error {
	rates, err := s.fetchRatesFromfrankfurter(ctx)
	if err != nil {
		return err
	}
	if err := s.app.UpsertFXRates(ctx, rates); err != nil {
		return err
	}
	// Also try to upsert currencies, but don't fail the refresh if it errors.
	currencies, err := s.fetchCurrenciesFromfrankfurter(ctx)
	if err == nil {
		if err := s.app.UpsertFXCurrencies(ctx, currencies); err != nil {
			log.Printf("warn: fx upsert currencies failed (non-fatal): %v", err)
		}
	}
	s.mu.Lock()
	s.lastRefresh = time.Now().UnixMilli()
	s.refreshErr = nil
	s.mu.Unlock()
	return nil
}

// Latest returns the most recent cached rate, or an error if not available.
// For USD/IRT, returns the operator-configured rate.
func (s *Service) Latest(base, quote money.CurrencyCode) (*RateSnapshot, error) {
	if base == quote {
		return &RateSnapshot{
			Base:        string(base),
			Quote:       string(quote),
			RateMicro:   1_000_000, // 1.0
			CapturedAtMs: time.Now().UnixMilli(),
			Source:      "frankfurter",
		}, nil
	}

	// Special case: USD/IRT uses operator config
	if base == money.USD && quote == money.IRT {
		s.mu.RLock()
		rate := s.irt_per_usd
		lastRefresh := s.lastRefresh
		s.mu.RUnlock()
		return &RateSnapshot{
			Base:        string(base),
			Quote:       string(quote),
			RateMicro:   rate,
			CapturedAtMs: lastRefresh,
			Source:      "operator",
		}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	key := rateKey(base, quote)
	rec, ok := s.rates[key]
	if !ok {
		return nil, fmt.Errorf("fx: no rate for %s/%s", base, quote)
	}

	// Convert string rate to int64 micro
	rateMicro, err := decimalToMicro(rec.Rate)
	if err != nil {
		return nil, fmt.Errorf("fx: invalid rate format for %s/%s: %w", base, quote, err)
	}

	return &RateSnapshot{
		Base:        string(rec.BaseCurrency),
		Quote:       string(rec.QuoteCurrency),
		RateMicro:   rateMicro,
		CapturedAtMs: rec.FetchedAt,
		Source:      rec.Provider,
	}, nil
}

// ListCurrencies returns all cached currencies, sorted by code.
func (s *Service) ListCurrencies() []runtimemodel.FXCurrencyRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]runtimemodel.FXCurrencyRecord, 0, len(s.currencies))
	for _, item := range s.currencies {
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b runtimemodel.FXCurrencyRecord) int {
		return strings.Compare(string(a.Code), string(b.Code))
	})
	return out
}

// Snapshot returns a debug snapshot of the FX service state.
func (s *Service) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	loaded := len(s.rates) > 0 || len(s.currencies) > 0
	ageMs := int64(0)
	if s.lastRefresh > 0 {
		ageMs = time.Now().UnixMilli() - s.lastRefresh
	}
	stale := loaded && ageMs > int64(24*time.Hour/time.Millisecond)

	return map[string]any{
		"loaded":          loaded,
		"last_refresh_ms": s.lastRefresh,
		"stale":           stale,
		"age_ms":          ageMs,
		"rate_count":      len(s.rates),
	}
}

// RateSnapshot represents a single FX rate at a moment in time.
type RateSnapshot struct {
	Base        string
	Quote       string
	RateMicro   int64 // rate × 1e6
	CapturedAtMs int64
	Source      string // "frankfurter" | "operator" | "request"
}

// Helper to build a rate key
func rateKey(base, quote money.CurrencyCode) string {
	return string(base) + "/" + string(quote)
}

// decimalToMicro converts a decimal string (e.g., "60000.5") to int64 micro (60000500000).
// Precision is limited to 6 decimal places (the millionth).
func decimalToMicro(s string) (int64, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid decimal: %q", s)
	}

	integer := parts[0]
	if integer == "" {
		integer = "0"
	}
	intVal := int64(0)
	for _, ch := range integer {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid decimal: %q", s)
		}
		intVal = intVal*10 + int64(ch-'0')
	}

	fractional := ""
	if len(parts) == 2 {
		fractional = parts[1]
	}

	// Pad or truncate to 6 decimal places
	for len(fractional) < 6 {
		fractional += "0"
	}
	if len(fractional) > 6 {
		fractional = fractional[:6]
	}

	fracVal := int64(0)
	for _, ch := range fractional {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid decimal: %q", s)
		}
		fracVal = fracVal*10 + int64(ch-'0')
	}

	return intVal*1_000_000 + fracVal, nil
}
