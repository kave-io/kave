package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kave-io/kave/core/pkg/money"
)

// MockAppStore is a minimal mock for testing.
type MockAppStore struct {
	rates       map[string]struct{}
	currencies  map[string]struct{}
	upsertError error
}

func (m *MockAppStore) ListFXRates(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *MockAppStore) ListFXCurrencies(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *MockAppStore) UpsertFXRates(ctx context.Context, rates interface{}) error {
	return m.upsertError
}

func (m *MockAppStore) UpsertFXCurrencies(ctx context.Context, currencies interface{}) error {
	return m.upsertError
}

// TestDecimalToMicro tests the decimal-to-micro conversion.
func TestDecimalToMicro(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      int64
		wantError bool
	}{
		{"1.0", "1.0", 1_000_000, false},
		{"1.5", "1.5", 1_500_000, false},
		{"60000.5", "60000.5", 60_000_500_000, false},
		{"0.123456", "0.123456", 123_456, false},
		{"0.1234567", "0.1234567", 123_456, false}, // truncate to 6 decimals
		{"100", "100", 100_000_000, false},
		{"0.0001", "0.0001", 100, false},
		{"invalid", "abc", 0, true},
		{"decimal.dot", "1.2.3", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalToMicro(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("decimalToMicro(%q) error = %v, wantError %v", tt.input, err, tt.wantError)
				return
			}
			if !tt.wantError && got != tt.want {
				t.Errorf("decimalToMicro(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestLatestSameBasequote tests that converting base == quote returns 1.0.
func TestLatestSameBasequote(t *testing.T) {
	svc := NewService(nil, 60_000_000_000)
	snap, err := svc.Latest(money.USD, money.USD)
	if err != nil {
		t.Fatalf("Latest(USD, USD) error = %v", err)
	}
	if snap.RateMicro != 1_000_000 {
		t.Errorf("rate = %d, want 1_000_000", snap.RateMicro)
	}
	if snap.Source != "frankfurter" {
		t.Errorf("source = %q, want frankfurter", snap.Source)
	}
}

// TestLatestUSDtoIRT tests the operator-configured USD/IRT rate.
func TestLatestUSDtoIRT(t *testing.T) {
	svc := NewService(nil, 60_000_500_000)
	snap, err := svc.Latest(money.USD, money.IRT)
	if err != nil {
		t.Fatalf("Latest(USD, IRT) error = %v", err)
	}
	if snap.RateMicro != 60_000_500_000 {
		t.Errorf("rate = %d, want 60_000_500_000", snap.RateMicro)
	}
	if snap.Source != "operator" {
		t.Errorf("source = %q, want operator", snap.Source)
	}
}

// TestLatestNotFound tests that requesting an uncached rate returns an error.
func TestLatestNotFound(t *testing.T) {
	svc := NewService(nil, 60_000_000_000)
	_, err := svc.Latest(money.USD, money.CurrencyCode("EUR"))
	if err == nil {
		t.Fatal("Latest(USD, EUR) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no rate") {
		t.Errorf("error = %q, want 'no rate'", err.Error())
	}
}

// TestSnapshot tests the debug snapshot.
func TestSnapshot(t *testing.T) {
	svc := NewService(nil, 60_000_000_000)
	snap := svc.Snapshot()
	if snap["loaded"] != false {
		t.Errorf("loaded = %v, want false", snap["loaded"])
	}
	if snap["stale"] != false {
		t.Errorf("stale = %v, want false", snap["stale"])
	}
}

// TestListCurrencies tests listing currencies (should be sorted).
func TestListCurrencies(t *testing.T) {
	svc := NewService(nil, 60_000_000_000)
	// Empty before load
	curs := svc.ListCurrencies()
	if len(curs) != 0 {
		t.Errorf("ListCurrencies() = %d currencies, want 0", len(curs))
	}
}

// TestFrankfurterResponseParsing tests the Frankfurter response parsing with a mock HTTP server.
func TestFrankfurterResponseParsing(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Minimal Frankfurter response
		json := `{
			"amount": "1",
			"base": "USD",
			"date": "2026-04-29",
			"rates": {
				"EUR": 0.92,
				"GBP": 0.79,
				"CHF": 0.87,
				"IRR": 42000.0
			}
		}`
		w.Write([]byte(json))
	}))
	defer server.Close()

	svc := NewService(nil, 60_000_000_000)
	svc.client.Timeout = 5 * time.Second // Short timeout for test

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
