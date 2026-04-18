package fx

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kave-io/kave/core/pkg/money"
)

func RegisterRoutes(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("/api/v1/fx/currencies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"currencies": svc.ListCurrencies()})
	})

	mux.HandleFunc("/api/v1/fx/rates", func(w http.ResponseWriter, r *http.Request) {
		base := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base"))))
		if base == "" {
			base = money.USD
		}
		quotes := splitCodes(r.URL.Query().Get("quotes"))
		if len(quotes) == 0 {
			quotes = []money.CurrencyCode{money.EUR, money.IRR, money.IRT}
		}
		var out []any
		for _, quote := range quotes {
			rate, err := svc.GetRate(base, quote)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			out = append(out, rate)
		}
		writeJSON(w, http.StatusOK, map[string]any{"rates": out})
	})

	mux.HandleFunc("/api/v1/fx/convert", func(w http.ResponseWriter, r *http.Request) {
		from := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("from"))))
		to := money.CurrencyCode(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("to"))))
		amount := r.URL.Query().Get("amount")
		if from == "" || to == "" || amount == "" {
			http.Error(w, "from, to, and amount are required", http.StatusBadRequest)
			return
		}
		converted, rate, err := svc.Convert(amount, from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"input":  map[string]any{"amount": amount, "currency": from},
			"output": converted,
			"rate":   rate,
		})
	})

	mux.HandleFunc("/api/v1/fx/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := svc.Refresh(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := svc.Load(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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
