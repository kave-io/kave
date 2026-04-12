package mappers

import runtimemodel "github.com/kave-io/kave/core/model/runtime"

// AppPriceModel is an app-layer pricing entry shape.
type AppPriceModel struct {
	Provider             string
	Match                string
	Source               string
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// AppPriceBook is an app-layer pricing view.
type AppPriceBook struct {
	Version string
	Entries []AppPriceModel
}

// ModelPriceBookToApp converts runtimemodel.PriceBook to app-layer pricing view.
func ModelPriceBookToApp(book *runtimemodel.PriceBook) *AppPriceBook {
	if book == nil {
		return nil
	}

	entries := make([]AppPriceModel, 0, len(book.Entries))
	for _, e := range book.Entries {
		entries = append(entries, AppPriceModel{
			Provider:             e.Provider,
			Match:                e.Match,
			Source:               e.Source,
			InputPerMillion:      e.InputPerMillion,
			OutputPerMillion:     e.OutputPerMillion,
			CacheReadPerMillion:  e.CacheReadPerMillion,
			CacheWritePerMillion: e.CacheWritePerMillion,
		})
	}

	return &AppPriceBook{
		Version: book.Version,
		Entries: entries,
	}
}

// AppPriceBookToModel converts app-layer pricing view to runtimemodel.PriceBook.
func AppPriceBookToModel(book *AppPriceBook) *runtimemodel.PriceBook {
	if book == nil {
		return nil
	}

	entries := make([]runtimemodel.PriceModel, 0, len(book.Entries))
	for _, e := range book.Entries {
		entries = append(entries, runtimemodel.PriceModel{
			Provider:             e.Provider,
			Match:                e.Match,
			Source:               e.Source,
			InputPerMillion:      e.InputPerMillion,
			OutputPerMillion:     e.OutputPerMillion,
			CacheReadPerMillion:  e.CacheReadPerMillion,
			CacheWritePerMillion: e.CacheWritePerMillion,
		})
	}

	return &runtimemodel.PriceBook{
		Version: book.Version,
		Entries: entries,
	}
}
