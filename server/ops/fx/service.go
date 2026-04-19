package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

const (
	defaultBaseCurrency = money.EUR
	defaultProvider     = "frankfurter"
)

var supportedCurrencies = []money.CurrencyCode{
	money.USD,
	money.EUR,
	money.GBP,
	money.CHF,
	money.IRR,
	money.IRT,
}

type Service struct {
	app      store.AppStore
	client   *http.Client
	baseURL  string
	interval time.Duration
	reloadCh chan time.Duration

	mu         sync.RWMutex
	rates      map[string]runtimemodel.FXRateRecord
	currencies map[money.CurrencyCode]runtimemodel.FXCurrencyRecord
}

func NewService(app store.AppStore, interval time.Duration) *Service {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Service{
		app:        app,
		client:     &http.Client{Timeout: 15 * time.Second},
		baseURL:    "https://api.frankfurter.dev/v2",
		interval:   interval,
		reloadCh:   make(chan time.Duration, 1),
		rates:      make(map[string]runtimemodel.FXRateRecord),
		currencies: make(map[money.CurrencyCode]runtimemodel.FXCurrencyRecord),
	}
}

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

func (s *Service) EnsureFresh(ctx context.Context) error {
	s.mu.RLock()
	hasRates := len(s.rates) > 0
	s.mu.RUnlock()
	if hasRates {
		return nil
	}
	if err := s.Refresh(ctx); err != nil {
		return err
	}
	return s.Load(ctx)
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		s.mu.RLock()
		interval := s.interval
		s.mu.RUnlock()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.Refresh(context.Background())
				_ = s.Load(context.Background())
			case newInterval := <-s.reloadCh:
				if newInterval <= 0 {
					continue
				}
				ticker.Stop()
				ticker = time.NewTicker(newInterval)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// SetInterval updates the refresh cadence for the background loop.
func (s *Service) SetInterval(interval time.Duration) {
	if interval <= 0 {
		return
	}
	s.mu.Lock()
	s.interval = interval
	s.mu.Unlock()
	select {
	case s.reloadCh <- interval:
	default:
	}
}

// Snapshot summarizes the cached FX state for status and doctor probes.
func (s *Service) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	loaded := len(s.rates) > 0 || len(s.currencies) > 0
	var lastRefreshMs int64
	for _, rate := range s.rates {
		if rate.FetchedAt > lastRefreshMs {
			lastRefreshMs = rate.FetchedAt
		}
	}
	for _, item := range s.currencies {
		if item.FetchedAt > lastRefreshMs {
			lastRefreshMs = item.FetchedAt
		}
	}

	ageMs := int64(0)
	if lastRefreshMs > 0 {
		ageMs = time.Now().UnixMilli() - lastRefreshMs
	}

	return map[string]any{
		"loaded":           loaded,
		"last_refresh_ms":  lastRefreshMs,
		"stale":            loaded && ageMs > int64(24*time.Hour/time.Millisecond),
		"age_ms":           ageMs,
		"refresh_interval": s.interval.Milliseconds(),
	}
}

func (s *Service) Refresh(ctx context.Context) error {
	currencies, err := s.fetchCurrencies(ctx)
	if err != nil {
		return err
	}
	rates, err := s.fetchRates(ctx)
	if err != nil {
		return err
	}
	if err := s.app.UpsertFXCurrencies(ctx, currencies); err != nil {
		return err
	}
	if err := s.app.UpsertFXRates(ctx, rates); err != nil {
		return err
	}
	return nil
}

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

func (s *Service) GetRate(base, quote money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	if base == quote {
		now := time.Now().UTC()
		return &runtimemodel.FXRateRecord{
			BaseCurrency:  base,
			QuoteCurrency: quote,
			Rate:          "1",
			Provider:      defaultProvider,
			AsOfDate:      now.Format("2006-01-02"),
			FetchedAt:     now.UnixMilli(),
		}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.rates[rateKey(base, quote)]
	if !ok {
		return nil, fmt.Errorf("fx rate not found: %s/%s", base, quote)
	}
	return &item, nil
}

func (s *Service) Convert(amount string, from, to money.CurrencyCode) (*money.Money, *runtimemodel.FXRateRecord, error) {
	src, err := money.Parse(amount, from)
	if err != nil {
		return nil, nil, err
	}
	rate, err := s.GetRate(from, to)
	if err != nil {
		return nil, nil, err
	}
	convertedAmount, err := applyRate(src.Amount, rate.Rate)
	if err != nil {
		return nil, nil, err
	}
	out, err := money.NewMoney(convertedAmount, to)
	if err != nil {
		return nil, nil, err
	}
	return &out, rate, nil
}

func (s *Service) fetchCurrencies(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/currencies", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frankfurter currencies: %s", strings.TrimSpace(string(body)))
	}

	type currencyResponse struct {
		ISOCode string  `json:"iso_code"`
		Name    string  `json:"name"`
		Symbol  *string `json:"symbol"`
	}
	var payload []currencyResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	now := time.Now().UTC().UnixMilli()
	allowed := make(map[money.CurrencyCode]struct{}, len(supportedCurrencies))
	for _, code := range supportedCurrencies {
		allowed[code] = struct{}{}
	}

	var out []runtimemodel.FXCurrencyRecord
	for _, item := range payload {
		code := money.CurrencyCode(strings.ToUpper(item.ISOCode))
		if _, ok := allowed[code]; !ok {
			continue
		}
		symbol := ""
		if item.Symbol != nil {
			symbol = *item.Symbol
		}
		out = append(out, runtimemodel.FXCurrencyRecord{
			Code:      code,
			Name:      item.Name,
			Symbol:    symbol,
			FetchedAt: now,
		})
	}

	out = append(out, runtimemodel.FXCurrencyRecord{
		Code:      money.IRT,
		Name:      "Iranian Toman",
		Symbol:    "T",
		FetchedAt: now,
	})
	return out, nil
}

func (s *Service) fetchRates(ctx context.Context) ([]runtimemodel.FXRateRecord, error) {
	quotes := []string{string(money.USD), string(money.GBP), string(money.CHF), string(money.IRR)}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/rates?base="+string(defaultBaseCurrency)+"&quotes="+strings.Join(quotes, ","), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frankfurter rates: %s", strings.TrimSpace(string(body)))
	}

	type rateResponse struct {
		Date  string      `json:"date"`
		Base  string      `json:"base"`
		Quote string      `json:"quote"`
		Rate  json.Number `json:"rate"`
	}

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var payload []rateResponse
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}

	pivot := map[money.CurrencyCode]string{
		defaultBaseCurrency: "1",
	}
	asOfDate := time.Now().UTC().Format("2006-01-02")
	for _, item := range payload {
		pivot[money.CurrencyCode(strings.ToUpper(item.Quote))] = item.Rate.String()
		asOfDate = item.Date
	}
	if irr, ok := pivot[money.IRR]; ok {
		rat, err := parseDecimalRat(irr)
		if err != nil {
			return nil, err
		}
		rat.Quo(rat, big.NewRat(10, 1))
		pivot[money.IRT] = formatRat(rat, 18)
	}

	now := time.Now().UTC().UnixMilli()
	var out []runtimemodel.FXRateRecord
	for _, base := range supportedCurrencies {
		baseRate, ok := pivot[base]
		if !ok {
			continue
		}
		baseRat, err := parseDecimalRat(baseRate)
		if err != nil {
			return nil, err
		}
		for _, quote := range supportedCurrencies {
			quoteRate, ok := pivot[quote]
			if !ok {
				continue
			}
			quoteRat, err := parseDecimalRat(quoteRate)
			if err != nil {
				return nil, err
			}
			rate := new(big.Rat).Quo(quoteRat, baseRat)
			out = append(out, runtimemodel.FXRateRecord{
				BaseCurrency:  base,
				QuoteCurrency: quote,
				Rate:          formatRat(rate, 18),
				Provider:      defaultProvider,
				AsOfDate:      asOfDate,
				FetchedAt:     now,
			})
		}
	}
	return out, nil
}

func applyRate(amount money.Amount, rate string) (money.Amount, error) {
	rat, err := parseDecimalRat(rate)
	if err != nil {
		return 0, err
	}
	value := new(big.Rat).Mul(new(big.Rat).SetInt64(amount.Nano()), rat)
	return ratToAmount(value)
}

func parseDecimalRat(s string) (*big.Rat, error) {
	rat, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", s)
	}
	return rat, nil
}

func formatRat(r *big.Rat, precision int) string {
	out := r.FloatString(precision)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		return "0"
	}
	return out
}

func ratToAmount(r *big.Rat) (money.Amount, error) {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	neg := num.Sign() < 0
	if neg {
		num.Neg(num)
	}
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("fx conversion overflow")
	}
	return money.Amount(q.Int64()), nil
}

func rateKey(base, quote money.CurrencyCode) string {
	return string(base) + "/" + string(quote)
}
