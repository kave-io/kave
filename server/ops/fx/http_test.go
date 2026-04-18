package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kave-io/kave/core/pkg/money"
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
			Rates []struct {
				BaseCurrency  money.CurrencyCode `json:"BaseCurrency"`
				QuoteCurrency money.CurrencyCode `json:"QuoteCurrency"`
				Rate          string             `json:"Rate"`
			} `json:"rates"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Rates) != 2 {
			t.Fatalf("len=%d", len(payload.Rates))
		}
		if payload.Rates[1].QuoteCurrency != money.IRT || payload.Rates[1].Rate != "5000" {
			t.Fatalf("unexpected rate %+v", payload.Rates[1])
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
			Output struct {
				Amount   string             `json:"amount"`
				Currency money.CurrencyCode `json:"currency"`
			} `json:"output"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Output.Amount != "15000" || payload.Output.Currency != money.IRT {
			t.Fatalf("unexpected output %+v", payload.Output)
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
	})

	t.Run("refresh post", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/fx/refresh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
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
	})

	t.Run("rates not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/fx/rates?base=USD&quotes=BAD", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}
