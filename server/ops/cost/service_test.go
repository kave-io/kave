package cost

import (
	"testing"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

func snap(inputPerM, outputPerM, cacheReadPerM, cacheWritePerM money.Amount) *runtimemodel.PriceSnapshot {
	return &runtimemodel.PriceSnapshot{
		InputPerMillion:      inputPerM,
		OutputPerMillion:     outputPerM,
		CacheReadPerMillion:  cacheReadPerM,
		CacheWritePerMillion: cacheWritePerM,
		Currency:             money.USD,
	}
}

func TestCalculate_nilSnapshot(t *testing.T) {
	got := Calculate(nil, 1000, 500, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0)
	if got != 0 {
		t.Fatalf("nil snapshot must return zero cost, got %v", got)
	}
}

func TestCalculate_inputOnly(t *testing.T) {
	// $3 per 1M input tokens; 1M tokens = $3
	s := snap(money.MustParseAmount("3.00"), 0, 0, 0)
	got := Calculate(s, 1_000_000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	want := money.MustParseAmount("3.00")
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCalculate_outputOnly(t *testing.T) {
	// $15 per 1M output; 500k tokens = $7.50
	s := snap(0, money.MustParseAmount("15.00"), 0, 0)
	got := Calculate(s, 0, 500_000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	want := money.MustParseAmount("7.50")
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCalculate_cacheReadCheaperThanInput(t *testing.T) {
	// cache-read priced at 10% of input: $0.30 vs $3.00
	s := snap(
		money.MustParseAmount("3.00"),
		money.MustParseAmount("15.00"),
		money.MustParseAmount("0.30"),
		money.MustParseAmount("3.75"),
	)
	// 1M cache-read should cost $0.30, 1M input should cost $3.00
	cacheOnly := Calculate(s, 0, 0, 1_000_000, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	inputOnly := Calculate(s, 1_000_000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if cacheOnly >= inputOnly {
		t.Fatalf("cache-read (%v) should be cheaper than input (%v)", cacheOnly, inputOnly)
	}
}

func TestCalculate_allZeroPrices(t *testing.T) {
	s := snap(0, 0, 0, 0)
	got := Calculate(s, 100, 200, 50, 25, 0, 0, 0, 0, 0, 0, 0, 0)
	if got != 0 {
		t.Fatalf("all-zero prices should yield zero cost, got %v", got)
	}
}

func TestCalculate_perRequest(t *testing.T) {
	// $0.001 per request, 10 requests = $0.01
	s := &runtimemodel.PriceSnapshot{
		PerRequest: money.MustParseAmount("0.001"),
		Currency:   money.USD,
	}
	got := Calculate(s, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0)
	want := money.MustParseAmount("0.01")
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCalculate_imageUnits(t *testing.T) {
	// $0.002 per image unit, 5 images = $0.01
	s := &runtimemodel.PriceSnapshot{
		ImageUnitPrice: money.MustParseAmount("0.002"),
		Currency:       money.USD,
	}
	got := Calculate(s, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0)
	want := money.MustParseAmount("0.01")
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCalculate_combined(t *testing.T) {
	s := &runtimemodel.PriceSnapshot{
		InputPerMillion:  money.MustParseAmount("3.00"),
		OutputPerMillion: money.MustParseAmount("15.00"),
		PerRequest:       money.MustParseAmount("0.001"),
		Currency:         money.USD,
	}
	// 100k input = $0.30, 50k output = $0.75, 1 request = $0.001
	got := Calculate(s, 100_000, 50_000, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0)
	want := money.MustParseAmount("1.051")
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCalculate_zeroTokens_noContribution(t *testing.T) {
	// Priced snapshot but zero tokens — must be zero, not a rounding artifact
	s := snap(
		money.MustParseAmount("3.00"),
		money.MustParseAmount("15.00"),
		money.MustParseAmount("0.30"),
		money.MustParseAmount("3.75"),
	)
	got := Calculate(s, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if got != 0 {
		t.Fatalf("zero usage should yield zero cost, got %v", got)
	}
}

func TestDefaultBook_loads(t *testing.T) {
	book, err := DefaultBook()
	if err != nil {
		t.Fatal(err)
	}
	if book == nil {
		t.Fatal("DefaultBook returned nil")
	}
	if len(book.Entries) == 0 {
		t.Fatal("DefaultBook must have at least one price entry")
	}
}

func TestDefaultBook_hasNonZeroPrices(t *testing.T) {
	book, _ := DefaultBook()
	allZero := true
	for _, e := range book.Entries {
		if e.InputPerMillion != 0 || e.OutputPerMillion != 0 || e.PerRequest != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("DefaultBook should have at least one entry with non-zero pricing")
	}
}

func TestValidateCurrencies_rejectsUnknown(t *testing.T) {
	book := &runtimemodel.PriceBook{
		Entries: []runtimemodel.PriceModel{
			{Provider: "openai", Match: "gpt-4", Currency: "EUR"},
		},
	}
	if err := validateCurrencies(book); err == nil {
		t.Fatal("expected error for unsupported currency EUR")
	}
}

func TestValidateCurrencies_acceptsUSDAndIRT(t *testing.T) {
	book := &runtimemodel.PriceBook{
		Entries: []runtimemodel.PriceModel{
			{Provider: "openai", Match: "gpt-4", Currency: money.USD},
			{Provider: "local", Match: "llama", Currency: money.IRT},
		},
	}
	if err := validateCurrencies(book); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeBook_setsDefaults(t *testing.T) {
	book := &runtimemodel.PriceBook{
		Entries: []runtimemodel.PriceModel{
			{Provider: " OpenAI ", Match: " GPT-4 "},
		},
	}
	out := normalizeBook(book)
	if out.Version != "custom" {
		t.Fatalf("expected version=custom, got %q", out.Version)
	}
	if out.Entries[0].Provider != "openai" {
		t.Fatalf("provider not lowercased/trimmed: %q", out.Entries[0].Provider)
	}
	if out.Entries[0].Match != "gpt-4" {
		t.Fatalf("match not lowercased/trimmed: %q", out.Entries[0].Match)
	}
	if out.Entries[0].Currency != money.USD {
		t.Fatalf("expected USD default currency, got %q", out.Entries[0].Currency)
	}
	if out.Entries[0].Source != "custom" {
		t.Fatalf("expected source=custom, got %q", out.Entries[0].Source)
	}
}

func TestNormalizeBook_nilSafe(t *testing.T) {
	out := normalizeBook(nil)
	if out == nil {
		t.Fatal("normalizeBook(nil) must not return nil")
	}
	if out.Entries == nil {
		t.Fatal("normalizeBook(nil) must return non-nil entries slice")
	}
}
