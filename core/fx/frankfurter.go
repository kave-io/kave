package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

// fetchRatesFromfrankfurter retrieves the latest USD/IRT rate from Frankfurter.
// IRT (Toman) is derived from IRR (Rial) returned by Frankfurter by dividing by 10.
func (s *Service) fetchRatesFromfrankfurter(ctx context.Context) ([]runtimemodel.FXRateRecord, error) {
	// Frankfurter endpoint: GET https://api.frankfurter.app/latest?from=USD&to=IRR
	url := "https://api.frankfurter.app/latest?from=USD&to=IRR"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frankfurter request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frankfurter returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	type rateResponse struct {
		Amount   string             `json:"amount"`
		Base     string             `json:"base"`
		Date     string             `json:"date"`
		Rates    map[string]float64 `json:"rates"`
	}

	var payload rateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("frankfurter decode failed: %w", err)
	}

	// Build pivot: all rates relative to USD
	pivot := map[money.CurrencyCode]string{
		money.USD: "1", // 1 USD = 1 USD
	}
	for quote, rate := range payload.Rates {
		pivot[money.CurrencyCode(strings.ToUpper(quote))] = fmt.Sprintf("%.18f", rate)
	}

	const irr money.CurrencyCode = "IRR"
	// Derive IRT (Toman) from IRR (Rial): 1 USD = X IRR → 1 USD = X/10 IRT (milli-Toman)
	if irrRate, ok := pivot[irr]; ok {
		var irrVal float64
		fmt.Sscanf(irrRate, "%f", &irrVal)
		pivot[money.IRT] = fmt.Sprintf("%.18f", irrVal/10.0)
	}

	now := time.Now().UTC().UnixMilli()
	var out []runtimemodel.FXRateRecord
	supportedCurrencies := []money.CurrencyCode{
		money.USD,
		money.IRT,
	}

	for _, base := range supportedCurrencies {
		baseRate, ok := pivot[base]
		if !ok {
			continue
		}
		for _, quote := range supportedCurrencies {
			quoteRate, ok := pivot[quote]
			if !ok {
				continue
			}
			var baseVal, quoteVal float64
			fmt.Sscanf(baseRate, "%f", &baseVal)
			fmt.Sscanf(quoteRate, "%f", &quoteVal)

			if baseVal == 0 {
				continue
			}
			rate := quoteVal / baseVal
			rateStr := fmt.Sprintf("%.18f", rate)

			out = append(out, runtimemodel.FXRateRecord{
				BaseCurrency:  base,
				QuoteCurrency: quote,
				Rate:          rateStr,
				Provider:      "frankfurter",
				AsOfDate:      payload.Date,
				FetchedAt:     now,
			})
		}
	}

	return out, nil
}

// fetchCurrenciesFromfrankfurter returns the static list of supported currencies.
func (s *Service) fetchCurrenciesFromfrankfurter(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	now := time.Now().UTC().UnixMilli()
	return []runtimemodel.FXCurrencyRecord{
		{Code: money.USD, Name: "US Dollar", Symbol: "$", FetchedAt: now},
		{Code: money.IRT, Name: "Iranian Toman", Symbol: "T", FetchedAt: now},
	}, nil
}
