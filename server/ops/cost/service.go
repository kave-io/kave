package cost

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kave-io/kave/core/mappers"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
)

//go:embed defaults.json
var defaultBookJSON []byte

type Service struct {
	app  store.AppStore
	mu   sync.RWMutex
	book *runtimemodel.PriceBook
}

func NewService(ctx context.Context, app store.AppStore) (*Service, error) {
	s := &Service{app: app}
	if err := s.Load(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Load(ctx context.Context) error {
	book, err := s.app.GetPriceBook(ctx)
	if err != nil {
		return err
	}
	if book == nil || len(book.Entries) == 0 || allZeroPricing(book) {
		book, err = DefaultBook()
		if err != nil {
			return err
		}
		if err := s.app.SavePriceBook(ctx, book); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.book = normalizeBook(book)
	s.mu.Unlock()
	return nil
}

func (s *Service) Current() *runtimemodel.PriceBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneBook(s.book)
}

func (s *Service) Replace(ctx context.Context, book *runtimemodel.PriceBook) error {
	book = normalizeBook(book)
	if err := validateCurrencies(book); err != nil {
		return err
	}
	if err := s.app.SavePriceBook(ctx, book); err != nil {
		return err
	}
	s.mu.Lock()
	s.book = cloneBook(book)
	s.mu.Unlock()
	return nil
}

func (s *Service) Snapshot(provider, model string) *runtimemodel.PriceSnapshot {
	s.mu.RLock()
	book := s.book
	s.mu.RUnlock()
	if book == nil {
		return nil
	}

	provider = strings.ToLower(provider)
	model = strings.ToLower(model)
	for _, entry := range book.Entries {
		entryProvider := strings.ToLower(entry.Provider)
		entryMatch := strings.ToLower(entry.Match)
		if entryProvider != "" && entryProvider != provider {
			continue
		}
		if entryMatch == "" || strings.Contains(model, entryMatch) {
			return &runtimemodel.PriceSnapshot{
				Version:               book.Version,
				Provider:              provider,
				Model:                 model,
				Match:                 entry.Match,
				Source:                entry.Source,
				Currency:              entry.Currency,
				InputPerMillion:       entry.InputPerMillion,
				OutputPerMillion:      entry.OutputPerMillion,
				CacheReadPerMillion:   entry.CacheReadPerMillion,
				CacheWritePerMillion:  entry.CacheWritePerMillion,
				ReasoningPerMillion:   entry.ReasoningPerMillion,
				AudioInputPerMillion:  entry.AudioInputPerMillion,
				AudioOutputPerMillion: entry.AudioOutputPerMillion,
				ImageUnitPrice:        entry.ImageUnitPrice,
				PerRequest:            entry.PerRequest,
				PerComputeMs:          entry.PerComputeMs,
				PerGBStored:           entry.PerGBStored,
				PerGBTransferred:      entry.PerGBTransferred,
				ResolvedAt:            int64(timex.Now()),
			}
		}
	}
	return nil
}

func allZeroPricing(book *runtimemodel.PriceBook) bool {
	if book == nil {
		return true
	}
	for _, e := range book.Entries {
		if e.InputPerMillion != 0 || e.OutputPerMillion != 0 || e.PerRequest != 0 || e.ImageUnitPrice != 0 {
			return false
		}
	}
	return true
}

// Calculate computes cost from a snapshot and all billable token/usage dimensions.
func Calculate(snapshot *runtimemodel.PriceSnapshot, input, output, cacheRead, cacheWrite, reasoning, audioIn, audioOut, imageUnits int, requestCount int, computeMs int64, storageBytes int64, bandwidthBytes int64) money.Amount {
	if snapshot == nil {
		return 0
	}
	total := money.Amount(0)
	addScaled := func(price money.Amount, units, scale int64) {
		if price == 0 || units == 0 {
			return
		}
		v, err := price.MulRatio(units, scale, money.RoundHalfUp)
		if err == nil {
			total, _ = total.Add(v)
		}
	}
	addExact := func(price money.Amount, units int64) {
		if price == 0 || units == 0 {
			return
		}
		v, err := price.Mul(units)
		if err == nil {
			total, _ = total.Add(v)
		}
	}
	addScaled(snapshot.InputPerMillion, int64(input), 1_000_000)
	addScaled(snapshot.OutputPerMillion, int64(output), 1_000_000)
	addScaled(snapshot.CacheReadPerMillion, int64(cacheRead), 1_000_000)
	addScaled(snapshot.CacheWritePerMillion, int64(cacheWrite), 1_000_000)
	addScaled(snapshot.ReasoningPerMillion, int64(reasoning), 1_000_000)
	addScaled(snapshot.AudioInputPerMillion, int64(audioIn), 1_000_000)
	addScaled(snapshot.AudioOutputPerMillion, int64(audioOut), 1_000_000)
	addExact(snapshot.ImageUnitPrice, int64(imageUnits))
	addExact(snapshot.PerRequest, int64(requestCount))
	addExact(snapshot.PerComputeMs, computeMs)
	addScaled(snapshot.PerGBStored, storageBytes, 1<<30)
	addScaled(snapshot.PerGBTransferred, bandwidthBytes, 1<<30)
	return total
}

func DefaultBook() (*runtimemodel.PriceBook, error) {
	var appBook mappers.AppPriceBook
	if err := json.Unmarshal(defaultBookJSON, &appBook); err != nil {
		return nil, err
	}
	book := mappers.AppPriceBookToModel(&appBook)
	return normalizeBook(book), nil
}

// validateCurrencies rejects PriceBook entries with currencies other than USD or IRT (Toman).
func validateCurrencies(book *runtimemodel.PriceBook) error {
	for i, entry := range book.Entries {
		switch entry.Currency {
		case money.USD, money.IRT:
			// allowed
		default:
			return fmt.Errorf("price book entry %d (%s/%s): currency %q not supported in v1; use %q or %q",
				i, entry.Provider, entry.Match, entry.Currency, money.USD, money.IRT)
		}
	}
	return nil
}

func normalizeBook(book *runtimemodel.PriceBook) *runtimemodel.PriceBook {
	if book == nil {
		return &runtimemodel.PriceBook{Entries: []runtimemodel.PriceModel{}}
	}
	out := cloneBook(book)
	if out.Version == "" {
		out.Version = "custom"
	}
	if out.Entries == nil {
		out.Entries = []runtimemodel.PriceModel{}
	}
	for i := range out.Entries {
		out.Entries[i].Provider = strings.ToLower(strings.TrimSpace(out.Entries[i].Provider))
		out.Entries[i].Match = strings.ToLower(strings.TrimSpace(out.Entries[i].Match))
		if out.Entries[i].Currency == "" {
			out.Entries[i].Currency = money.USD
		}
		if out.Entries[i].Source == "" {
			out.Entries[i].Source = "custom"
		}
	}
	return out
}

func cloneBook(book *runtimemodel.PriceBook) *runtimemodel.PriceBook {
	if book == nil {
		return nil
	}
	out := &runtimemodel.PriceBook{
		Version: book.Version,
		Entries: make([]runtimemodel.PriceModel, len(book.Entries)),
	}
	copy(out.Entries, book.Entries)
	return out
}
