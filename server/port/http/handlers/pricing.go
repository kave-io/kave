package handlers

import (
	"net/http"

	"github.com/kave-io/kave/core/store"
)

type priceModelResp struct {
	Provider             string  `json:"provider"`
	Match                string  `json:"match"`
	Source               string  `json:"source"`
	InputPerMillion      float64 `json:"input_per_million"`
	OutputPerMillion     float64 `json:"output_per_million"`
	CacheReadPerMillion  float64 `json:"cache_read_per_million"`
	CacheWritePerMillion float64 `json:"cache_write_per_million"`
}

type priceBookResp struct {
	Version string           `json:"version"`
	Entries []priceModelResp `json:"entries"`
}

type putPricingReq struct {
	Version string           `json:"version"`
	Entries []priceModelResp `json:"entries"`
}

func (a *API) getPricing(w http.ResponseWriter, r *http.Request) {
	book := a.prices.Current()
	if book == nil {
		writeError(w, http.StatusNotFound, "price book not found")
		return
	}
	writeJSON(w, http.StatusOK, toPriceBookResp(book))
}

func (a *API) putPricing(w http.ResponseWriter, r *http.Request) {
	var req putPricingReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Entries) == 0 {
		writeError(w, http.StatusBadRequest, "entries are required")
		return
	}

	book := &store.PriceBook{
		Version: req.Version,
		Entries: make([]store.PriceModel, len(req.Entries)),
	}
	for i, entry := range req.Entries {
		book.Entries[i] = store.PriceModel{
			Provider:             entry.Provider,
			Match:                entry.Match,
			Source:               entry.Source,
			InputPerMillion:      entry.InputPerMillion,
			OutputPerMillion:     entry.OutputPerMillion,
			CacheReadPerMillion:  entry.CacheReadPerMillion,
			CacheWritePerMillion: entry.CacheWritePerMillion,
		}
	}

	if err := a.prices.Replace(r.Context(), book); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPriceBookResp(a.prices.Current()))
}

func toPriceBookResp(book *store.PriceBook) priceBookResp {
	resp := priceBookResp{
		Version: book.Version,
		Entries: make([]priceModelResp, len(book.Entries)),
	}
	for i, entry := range book.Entries {
		resp.Entries[i] = priceModelResp{
			Provider:             entry.Provider,
			Match:                entry.Match,
			Source:               entry.Source,
			InputPerMillion:      entry.InputPerMillion,
			OutputPerMillion:     entry.OutputPerMillion,
			CacheReadPerMillion:  entry.CacheReadPerMillion,
			CacheWritePerMillion: entry.CacheWritePerMillion,
		}
	}
	return resp
}
