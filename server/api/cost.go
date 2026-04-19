package api

import (
	"encoding/json"
	"net/http"

	"github.com/kave-io/kave/core/mappers"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/server/internal/contract"
)

// getCostSummary returns a spend report.
func (h *Handler) getCostSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	filter := &runtimemodel.SpendFilter{
		AgentID:   getQueryParam(r, "agent_id"),
		Connector: getQueryParam(r, "connector"),
		Model:     getQueryParam(r, "model"),
	}

	report, err := h.app.GetSpendReport(ctx, filter)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.get_failed", err.Error())
		return
	}

	// Convert amount maps to string maps
	byAgent := make(map[string]string)
	for k, v := range report.ByAgent {
		byAgent[k] = v.String()
	}
	byConnector := make(map[string]string)
	for k, v := range report.ByConnector {
		byConnector[k] = v.String()
	}
	byModel := make(map[string]string)
	for k, v := range report.ByModel {
		byModel[k] = v.String()
	}

	// Convert to API types
	result := &SpendReport{
		Total:         contract.Money{Amount: report.Total.String(), Currency: apiDefaultCurrency},
		ByAgent:       byAgent,
		ByConnector:   byConnector,
		ByModel:       byModel,
		PeriodStart:   isoFromMS(report.PeriodStart),
		PeriodStartMS: report.PeriodStart,
		PeriodEnd:     isoFromMS(report.PeriodEnd),
		PeriodEndMS:   report.PeriodEnd,
	}
	if result.ByAgent == nil {
		result.ByAgent = map[string]string{}
	}
	if result.ByConnector == nil {
		result.ByConnector = map[string]string{}
	}
	if result.ByModel == nil {
		result.ByModel = map[string]string{}
	}

	responseJSON(w, http.StatusOK, "SpendReport", result, nil, nil)
}

// getPriceBook returns the current pricing configuration.
func (h *Handler) getPriceBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	book, err := h.app.GetPriceBook(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.get_failed", err.Error())
		return
	}

	if book == nil {
		errorJSON(w, http.StatusNotFound, "price_book.not_found", "price book not found")
		return
	}

	// Convert to API types using mapper
	appBook := mappers.ModelPriceBookToApp(book)
	result := &PriceBook{
		Version: appBook.Version,
		Entries: make([]PriceModel, len(appBook.Entries)),
	}
	for i, e := range appBook.Entries {
		result.Entries[i] = PriceModel{
			Provider:             e.Provider,
			Match:                e.Match,
			Source:               e.Source,
			Currency:             e.Currency,
			InputPerMillion:      e.InputPerMillion,
			OutputPerMillion:     e.OutputPerMillion,
			CacheReadPerMillion:  e.CacheReadPerMillion,
			CacheWritePerMillion: e.CacheWritePerMillion,
		}
	}

	responseJSON(w, http.StatusOK, "PriceBook", result, nil, nil)
}

// updatePriceBook updates the pricing configuration.
func (h *Handler) updatePriceBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req PriceBook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "request.invalid", "invalid request body")
		return
	}

	// Convert from API format to app format
	appBook := &mappers.AppPriceBook{
		Version: req.Version,
		Entries: make([]mappers.AppPriceModel, len(req.Entries)),
	}
	for i, e := range req.Entries {
		appBook.Entries[i] = mappers.AppPriceModel{
			Provider:             e.Provider,
			Match:                e.Match,
			Source:               e.Source,
			Currency:             e.Currency,
			InputPerMillion:      e.InputPerMillion,
			OutputPerMillion:     e.OutputPerMillion,
			CacheReadPerMillion:  e.CacheReadPerMillion,
			CacheWritePerMillion: e.CacheWritePerMillion,
		}
	}

	// Convert to runtime model using mapper
	book := mappers.AppPriceBookToModel(appBook)

	// Update in store
	if err := h.app.SavePriceBook(ctx, book); err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.update_failed", err.Error())
		return
	}

	responseJSON(w, http.StatusOK, "PriceBook", req, nil, nil)
}
