package fx

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kave-io/kave/core/pkg/money"
	sqlitestore "github.com/kave-io/kave/server/internal/store/sqlite"
)

func TestApplyRate(t *testing.T) {
	got, err := applyRate(money.MustParseAmount("10"), "1.25")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "12.5" {
		t.Fatalf("got %s want 12.5", got)
	}
}

func TestApplyRateTomanPrecision(t *testing.T) {
	got, err := applyRate(money.MustParseAmount("1"), "450000.5")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "450000.5" {
		t.Fatalf("got %s want 450000.5", got)
	}
}

func TestFormatRat(t *testing.T) {
	if got := formatRat(mustRat("1.230000"), 6); got != "1.23" {
		t.Fatalf("got %q", got)
	}
}

func TestServiceRefreshLoadAndConvert(t *testing.T) {
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

	currencies := svc.ListCurrencies()
	if len(currencies) < 5 {
		t.Fatalf("expected currencies, got %d", len(currencies))
	}
	foundIRT := false
	for _, item := range currencies {
		if item.Code == money.IRT {
			foundIRT = true
			break
		}
	}
	if !foundIRT {
		t.Fatal("expected IRT in currency list")
	}

	rate, err := svc.GetRate(money.USD, money.IRT)
	if err != nil {
		t.Fatal(err)
	}
	if rate.Rate != "5000" {
		t.Fatalf("USD/IRT rate=%s want 5000", rate.Rate)
	}

	converted, appliedRate, err := svc.Convert("2", money.USD, money.IRT)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Amount.String() != "10000" {
		t.Fatalf("got %s want 10000", converted.Amount)
	}
	if appliedRate.Rate != "5000" {
		t.Fatalf("applied rate=%s", appliedRate.Rate)
	}

	persisted, err := app.GetFXRate(ctx, money.USD, money.IRT)
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || persisted.Rate != "5000" {
		t.Fatalf("persisted rate=%+v", persisted)
	}
}

func TestEnsureFreshOnlyRefreshesWhenCacheEmpty(t *testing.T) {
	ctx := context.Background()
	app := newSQLiteAppStore(t)
	svc := NewService(app, 0)

	var hits atomic.Int32
	upstream := newFrankfurterStub(t, &hits)
	svc.baseURL = upstream.URL
	svc.client = upstream.Client()

	if err := svc.EnsureFresh(ctx); err != nil {
		t.Fatal(err)
	}
	firstHits := hits.Load()
	if firstHits == 0 {
		t.Fatal("expected upstream calls")
	}

	if err := svc.EnsureFresh(ctx); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != firstHits {
		t.Fatalf("unexpected extra refresh: before=%d after=%d", firstHits, hits.Load())
	}
}

func TestGetRateIdentity(t *testing.T) {
	svc := NewService(nil, 0)
	rate, err := svc.GetRate(money.USD, money.USD)
	if err != nil {
		t.Fatal(err)
	}
	if rate.Rate != "1" {
		t.Fatalf("got %s", rate.Rate)
	}
}

func TestConvertErrors(t *testing.T) {
	svc := NewService(nil, 0)
	if _, _, err := svc.Convert("bad", money.USD, money.EUR); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := parseDecimalRat("bad"); err == nil {
		t.Fatal("expected rat parse error")
	}
}

func newSQLiteAppStore(t *testing.T) *sqlitestore.SQLiteAppStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fx.db")
	app, err := sqlitestore.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func newFrankfurterStub(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc("/currencies", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"iso_code": "USD", "name": "US Dollar", "symbol": "$"},
			{"iso_code": "EUR", "name": "Euro", "symbol": "€"},
			{"iso_code": "GBP", "name": "Pound Sterling", "symbol": "£"},
			{"iso_code": "CHF", "name": "Swiss Franc", "symbol": "CHF"},
			{"iso_code": "IRR", "name": "Iranian Rial", "symbol": "IRR"},
		})
	})
	handler.HandleFunc("/rates", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"date": "2026-04-17", "base": "EUR", "quote": "USD", "rate": "2"},
			{"date": "2026-04-17", "base": "EUR", "quote": "GBP", "rate": "0.8"},
			{"date": "2026-04-17", "base": "EUR", "quote": "CHF", "rate": "1.1"},
			{"date": "2026-04-17", "base": "EUR", "quote": "IRR", "rate": "100000"},
		})
	})
	return httptest.NewServer(handler)
}

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return r
}
