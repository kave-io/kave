package cost

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
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
	if book == nil || len(book.Entries) == 0 {
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
				Version:              book.Version,
				Provider:             provider,
				Model:                model,
				Match:                entry.Match,
				Source:               entry.Source,
				InputPerMillion:      entry.InputPerMillion,
				OutputPerMillion:     entry.OutputPerMillion,
				CacheReadPerMillion:  entry.CacheReadPerMillion,
				CacheWritePerMillion: entry.CacheWritePerMillion,
				ReasoningPerMillion:  entry.ReasoningPerMillion,
				AudioInputPerMillion: entry.AudioInputPerMillion,
				AudioOutputPerMillion: entry.AudioOutputPerMillion,
				ImageUnitPrice:       entry.ImageUnitPrice,
				PerRequest:           entry.PerRequest,
				PerComputeMs:         entry.PerComputeMs,
				PerGBStored:          entry.PerGBStored,
				PerGBTransferred:     entry.PerGBTransferred,
				ResolvedAt:           int64(timex.Now()),
			}
		}
	}
	return nil
}

// Calculate computes cost from a snapshot and all billable token/usage dimensions.
func Calculate(snapshot *runtimemodel.PriceSnapshot, input, output, cacheRead, cacheWrite, reasoning, audioIn, audioOut, imageUnits int, requestCount int, computeMs int64, storageBytes int64, bandwidthBytes int64) money.Amount {
	if snapshot == nil {
		return 0
	}
	total := float64(input)*snapshot.InputPerMillion +
		float64(output)*snapshot.OutputPerMillion +
		float64(cacheRead)*snapshot.CacheReadPerMillion +
		float64(cacheWrite)*snapshot.CacheWritePerMillion +
		float64(reasoning)*snapshot.ReasoningPerMillion +
		float64(audioIn)*snapshot.AudioInputPerMillion +
		float64(audioOut)*snapshot.AudioOutputPerMillion +
		float64(imageUnits)*snapshot.ImageUnitPrice
	total /= 1_000_000
	total += float64(requestCount) * snapshot.PerRequest
	if computeMs > 0 {
		total += float64(computeMs) * snapshot.PerComputeMs
	}
	if storageBytes > 0 {
		total += float64(storageBytes) / (1 << 30) * snapshot.PerGBStored
	}
	if bandwidthBytes > 0 {
		total += float64(bandwidthBytes) / (1 << 30) * snapshot.PerGBTransferred
	}
	return money.FromDollars(total)
}

func DefaultBook() (*runtimemodel.PriceBook, error) {
	var book runtimemodel.PriceBook
	if err := json.Unmarshal(defaultBookJSON, &book); err != nil {
		return nil, err
	}
	return normalizeBook(&book), nil
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
