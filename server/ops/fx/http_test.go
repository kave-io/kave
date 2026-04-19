package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/server/internal/contract"
)

func TestHTTPRoutesRatesAndConvert(t *testing.T) {
	ctx := context.Background()
	app := newSQLiteAppStore(t)
	svc := NewService(app, 0)

	upstream := newFrankfurterStub(t, nil)
	svc.baseURL = upstream.URL
	svc.client = upstream.Client()
	if err := svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Load(ctx); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, svc)

	t.Run("rates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/rates?base=USD&quotes=EUR,IRT", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			SchemaVersion int    `json:"schema_version"`
			Kind          string `json:"kind"`
			Data          []struct {
				BaseCurrency  string `json:"base_currency"`
				QuoteCurrency string `json:"quote_currency"`
				Rate          string `json:"rate"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.SchemaVersion != contract.SchemaVersion || payload.Kind != "FXRateList" {
			t.Fatalf("unexpected envelope: %+v", payload)
		}
		if len(payload.Data) != 2 {
			t.Fatalf("len=%d", len(payload.Data))
		}
		if payload.Data[1].QuoteCurrency != string(money.IRT) || payload.Data[1].Rate != "5000" {
			t.Fatalf("unexpected rate %+v", payload.Data[1])
		}
	})

	t.Run("convert", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/convert?from=USD&to=IRT&amount=3", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Kind string `json:"kind"`
			Data struct {
				Output struct {
					Amount   string             `json:"amount"`
					Currency money.CurrencyCode `json:"currency"`
				} `json:"output"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Kind != "FXConversion" {
			t.Fatalf("unexpected kind: %s", payload.Kind)
		}
		if payload.Data.Output.Amount != "15000" || payload.Data.Output.Currency != money.IRT {
			t.Fatalf("unexpected output %+v", payload.Data.Output)
		}
	})
}

func TestHTTPRoutesRefreshAndErrors(t *testing.T) {
	ctx := context.Background()
	app := newSQLiteAppStore(t)
	svc := NewService(app, 0)

	upstream := newFrankfurterStub(t, nil)
	svc.baseURL = upstream.URL
	svc.client = upstream.Client()

	mux := http.NewServeMux()
	RegisterRoutes(mux, svc)

	t.Run("refresh method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/refresh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("code=%d", rec.Code)
		}
		var payload struct {
			Kind  string `json:"kind"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Kind != "Error" || payload.Error.Code != "request.method_not_allowed" {
			t.Fatalf("unexpected error payload: %+v", payload)
		}
	})

	t.Run("refresh post", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/fx/refresh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Kind string `json:"kind"`
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Kind != "FXRefreshResult" || payload.Data.Status != "ok" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		rate, err := app.GetFXRate(ctx, money.USD, money.IRT)
		if err != nil {
			t.Fatal(err)
		}
		if rate == nil || rate.Rate != "5000" {
			t.Fatalf("rate=%+v", rate)
		}
	})

	t.Run("convert missing params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/convert?from=USD", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code=%d", rec.Code)
		}
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Error.Code != "request.invalid" {
			t.Fatalf("unexpected error code: %+v", payload)
		}
	})

	t.Run("rates not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/rates?base=USD&quotes=BAD", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Error.Code != "fx.rate_not_found" {
			t.Fatalf("unexpected error code: %+v", payload)
		}
	})
}
