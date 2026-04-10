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
)

//go:embed defaults.json
var defaultBookJSON []byte

type Service struct {
	app  store.AppStore
	mu   sync.RWMutex
	book *store.PriceBook
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

func (s *Service) Current() *store.PriceBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneBook(s.book)
}

func (s *Service) Replace(ctx context.Context, book *store.PriceBook) error {
	book = normalizeBook(book)
	if err := s.app.SavePriceBook(ctx, book); err != nil {
		return err
	}
	s.mu.Lock()
	s.book = cloneBook(book)
	s.mu.Unlock()
	return nil
}

func (s *Service) Snapshot(provider, model string) *store.PriceSnapshot {
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
			return &store.PriceSnapshot{
				Version:              book.Version,
				Provider:             provider,
				Model:                model,
				Match:                entry.Match,
				Source:               entry.Source,
				InputPerMillion:      entry.InputPerMillion,
				OutputPerMillion:     entry.OutputPerMillion,
				CacheReadPerMillion:  entry.CacheReadPerMillion,
				CacheWritePerMillion: entry.CacheWritePerMillion,
				ResolvedAt:           int64(timex.Now()),
			}
		}
	}
	return nil
}

func Calculate(snapshot *store.PriceSnapshot, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) money.Amount {
	if snapshot == nil {
		return 0
	}
	return money.FromDollars((float64(inputTokens)*snapshot.InputPerMillion +
		float64(outputTokens)*snapshot.OutputPerMillion +
		float64(cacheReadTokens)*snapshot.CacheReadPerMillion +
		float64(cacheWriteTokens)*snapshot.CacheWritePerMillion) / 1_000_000)
}

func DefaultBook() (*store.PriceBook, error) {
	var book store.PriceBook
	if err := json.Unmarshal(defaultBookJSON, &book); err != nil {
		return nil, err
	}
	return normalizeBook(&book), nil
}

func normalizeBook(book *store.PriceBook) *store.PriceBook {
	if book == nil {
		return &store.PriceBook{Entries: []store.PriceModel{}}
	}
	out := cloneBook(book)
	if out.Version == "" {
		out.Version = "custom"
	}
	if out.Entries == nil {
		out.Entries = []store.PriceModel{}
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

func cloneBook(book *store.PriceBook) *store.PriceBook {
	if book == nil {
		return nil
	}
	out := &store.PriceBook{
		Version: book.Version,
		Entries: make([]store.PriceModel, len(book.Entries)),
	}
	copy(out.Entries, book.Entries)
	return out
}
