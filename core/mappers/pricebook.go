package mappers

import (
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

// AppPriceModel is an app-layer pricing entry shape with JSON tags for external unmarshaling.
type AppPriceModel struct {
	Provider              string `json:"provider"`
	Match                 string `json:"match"`
	Source                string `json:"source"`
	Currency              string `json:"currency"`
	InputPerMillion       string `json:"input_per_million"`
	OutputPerMillion      string `json:"output_per_million"`
	CacheReadPerMillion   string `json:"cache_read_per_million"`
	CacheWritePerMillion  string `json:"cache_write_per_million"`
	ReasoningPerMillion   string `json:"reasoning_per_million"`
	AudioInputPerMillion  string `json:"audio_input_per_million"`
	AudioOutputPerMillion string `json:"audio_output_per_million"`
	ImageUnitPrice        string `json:"image_unit_price"`
	PerRequest            string `json:"per_request"`
	PerComputeMs          string `json:"per_compute_ms"`
	PerGBStored           string `json:"per_gb_stored"`
	PerGBTransferred      string `json:"per_gb_transferred"`
}

// AppPriceBook is an app-layer pricing view with JSON tags for external unmarshaling.
type AppPriceBook struct {
	Version string          `json:"version"`
	Entries []AppPriceModel `json:"entries"`
}

// ModelPriceBookToApp converts runtimemodel.PriceBook to app-layer pricing view.
func ModelPriceBookToApp(book *runtimemodel.PriceBook) *AppPriceBook {
	if book == nil {
		return nil
	}

	entries := make([]AppPriceModel, 0, len(book.Entries))
	for _, e := range book.Entries {
		entries = append(entries, AppPriceModel{
			Provider:              e.Provider,
			Match:                 e.Match,
			Source:                e.Source,
			Currency:              string(e.Currency),
			InputPerMillion:       e.InputPerMillion.String(),
			OutputPerMillion:      e.OutputPerMillion.String(),
			CacheReadPerMillion:   e.CacheReadPerMillion.String(),
			CacheWritePerMillion:  e.CacheWritePerMillion.String(),
			ReasoningPerMillion:   e.ReasoningPerMillion.String(),
			AudioInputPerMillion:  e.AudioInputPerMillion.String(),
			AudioOutputPerMillion: e.AudioOutputPerMillion.String(),
			ImageUnitPrice:        e.ImageUnitPrice.String(),
			PerRequest:            e.PerRequest.String(),
			PerComputeMs:          e.PerComputeMs.String(),
			PerGBStored:           e.PerGBStored.String(),
			PerGBTransferred:      e.PerGBTransferred.String(),
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
			Provider:              e.Provider,
			Match:                 e.Match,
			Source:                e.Source,
			Currency:              money.CurrencyCode(e.Currency),
			InputPerMillion:       parseAmount(e.InputPerMillion),
			OutputPerMillion:      parseAmount(e.OutputPerMillion),
			CacheReadPerMillion:   parseAmount(e.CacheReadPerMillion),
			CacheWritePerMillion:  parseAmount(e.CacheWritePerMillion),
			ReasoningPerMillion:   parseAmount(e.ReasoningPerMillion),
			AudioInputPerMillion:  parseAmount(e.AudioInputPerMillion),
			AudioOutputPerMillion: parseAmount(e.AudioOutputPerMillion),
			ImageUnitPrice:        parseAmount(e.ImageUnitPrice),
			PerRequest:            parseAmount(e.PerRequest),
			PerComputeMs:          parseAmount(e.PerComputeMs),
			PerGBStored:           parseAmount(e.PerGBStored),
			PerGBTransferred:      parseAmount(e.PerGBTransferred),
		})
	}

	return &runtimemodel.PriceBook{
		Version: book.Version,
		Entries: entries,
	}
}

func parseAmount(s string) money.Amount {
	if s == "" {
		return 0
	}
	amt, err := money.ParseAmount(s)
	if err != nil {
		return 0
	}
	return amt
}
