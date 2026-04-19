package fx

import (
	"net/http"
	"strings"
	"time"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/server/internal/contract"
)

func RegisterRoutes(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("/api/v1/fx/currencies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			errorJSON(w, http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed")
			return
		}
		items := svc.ListCurrencies()
		data := make([]fxCurrency, 0, len(items))
		for _, item := range items {
			data = append(data, fxCurrency{
				Code:        string(item.Code),
				Name:        item.Name,
				Symbol:      item.Symbol,
				FetchedAt:   isoFromMS(item.FetchedAt),
				FetchedAtMS: item.FetchedAt,
			})
		}
		responseJSON(w, http.StatusOK, "FXCurrencyList", data, &contract.Page{
			NextCursor: nil,
			Limit:      len(data),
		})
	})

	mux.HandleFunc("/api/v1/fx/rates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			errorJSON(w, http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed")
			return
		}
		base := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base"))))
		if base == "" {
			base = money.USD
		}
		quotes := splitCodes(r.URL.Query().Get("quotes"))
		if len(quotes) == 0 {
			quotes = []money.CurrencyCode{money.EUR, money.IRR, money.IRT}
		}
		out := make([]fxRate, 0, len(quotes))
		for _, quote := range quotes {
			rate, err := svc.GetRate(base, quote)
			if err != nil {
				errorJSON(w, http.StatusNotFound, "fx.rate_not_found", err.Error())
				return
			}
			out = append(out, fxRate{
				BaseCurrency:  string(rate.BaseCurrency),
				QuoteCurrency: string(rate.QuoteCurrency),
				Rate:          rate.Rate,
				Provider:      rate.Provider,
				AsOfDate:      rate.AsOfDate,
				FetchedAt:     isoFromMS(rate.FetchedAt),
				FetchedAtMS:   rate.FetchedAt,
			})
		}
		responseJSON(w, http.StatusOK, "FXRateList", out, &contract.Page{
			NextCursor: nil,
			Limit:      len(out),
		})
	})

	mux.HandleFunc("/api/v1/fx/convert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			errorJSON(w, http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed")
			return
		}
		from := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("from"))))
		to := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("to"))))
		amount := r.URL.Query().Get("amount")
		if from == "" || to == "" || amount == "" {
			errorJSON(w, http.StatusBadRequest, "request.invalid", "from, to, and amount are required")
			return
		}
		converted, rate, err := svc.Convert(amount, from, to)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, "fx.convert_failed", err.Error())
			return
		}
		responseJSON(w, http.StatusOK, "FXConversion", fxConversion{
			Input: contract.Money{
				Amount:   amount,
				Currency: string(from),
			},
			Output: contract.Money{
				Amount:   converted.Amount.String(),
				Currency: string(converted.Currency),
			},
			Rate: fxRate{
				BaseCurrency:  string(rate.BaseCurrency),
				QuoteCurrency: string(rate.QuoteCurrency),
				Rate:          rate.Rate,
				Provider:      rate.Provider,
				AsOfDate:      rate.AsOfDate,
				FetchedAt:     isoFromMS(rate.FetchedAt),
				FetchedAtMS:   rate.FetchedAt,
			},
		}, nil)
	})

	mux.HandleFunc("/api/v1/fx/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			errorJSON(w, http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed")
			return
		}
		if err := svc.Refresh(r.Context()); err != nil {
			errorJSON(w, http.StatusBadGateway, "fx.refresh_failed", err.Error())
			return
		}
		if err := svc.Load(r.Context()); err != nil {
			errorJSON(w, http.StatusInternalServerError, "fx.load_failed", err.Error())
			return
		}
		now := time.Now().UnixMilli()
		responseJSON(w, http.StatusOK, "FXRefreshResult", map[string]any{
			"status":          "ok",
			"refreshed_at":    isoFromMS(now),
			"refreshed_at_ms": now,
		}, nil)
	})
}

type fxCurrency struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Symbol      string `json:"symbol"`
	FetchedAt   string `json:"fetched_at"`
	FetchedAtMS int64  `json:"fetched_at_ms"`
}

type fxRate struct {
	BaseCurrency  string `json:"base_currency"`
	QuoteCurrency string `json:"quote_currency"`
	Rate          string `json:"rate"`
	Provider      string `json:"provider"`
	AsOfDate      string `json:"as_of_date"`
	FetchedAt     string `json:"fetched_at"`
	FetchedAtMS   int64  `json:"fetched_at_ms"`
}

type fxConversion struct {
	Input  contract.Money `json:"input"`
	Output contract.Money `json:"output"`
	Rate   fxRate         `json:"rate"`
}

func responseJSON(w http.ResponseWriter, status int, kind string, data any, page *contract.Page) {
	contract.WriteSuccess(w, status, kind, data, page, nil)
}

func errorJSON(w http.ResponseWriter, status int, code, message string) {
	contract.WriteError(w, status, code, message, nil)
}

func isoFromMS(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func splitCodes(raw string) []money.CurrencyCode {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]money.CurrencyCode, 0, len(parts))
	for _, part := range parts {
		part = strings.ToUpper(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		out = append(out, money.CurrencyCode(part))
	}
	return out
}
