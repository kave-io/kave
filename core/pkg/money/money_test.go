package money

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestLookupCurrency(t *testing.T) {
	cases := []struct {
		code string
		want CurrencyCode
		ok   bool
	}{
		{"usd", USD, true},
		{" IRT ", IRT, true},
		{"irt", IRT, true},
		{"xxx", "", false},
	}
	for _, tc := range cases {
		got, ok := LookupCurrency(tc.code)
		if ok != tc.ok {
			t.Fatalf("%q ok=%v want %v", tc.code, ok, tc.ok)
		}
		if ok && got.Code != tc.want {
			t.Fatalf("%q code=%s want %s", tc.code, got.Code, tc.want)
		}
	}
}

func TestValidateV1Currency(t *testing.T) {
	if err := ValidateV1Currency(USD); err != nil {
		t.Fatalf("USD: %v", err)
	}
	if err := ValidateV1Currency(IRT); err != nil {
		t.Fatalf("IRT: %v", err)
	}
	if err := ValidateV1Currency(CurrencyCode("EUR")); err == nil {
		t.Fatal("EUR should be rejected by v1 validator")
	}
	if err := ValidateV1Currency(CurrencyCode("TMN")); err == nil {
		t.Fatal("TMN should be rejected by v1 validator")
	}
}

func TestMustCurrencyPanicsOnUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = MustCurrency("BAD")
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want Amount
	}{
		{"0", 0},
		{"1", Dollar},
		{"1.25", Dollar + 250*MilliDollar},
		{"0.0000025", 2500},
		{"-10.5", -(10*Dollar + 500*MilliDollar)},
		{"+2.000000001", 2*Dollar + 1},
		{".5", 500 * MilliDollar},
		{"0003.50", 3*Dollar + 500*MilliDollar},
	}
	for _, tc := range cases {
		got, err := ParseAmount(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseAmountErrors(t *testing.T) {
	cases := []struct {
		in      string
		wantErr error
	}{
		{"", ErrInvalidAmount},
		{"-", ErrInvalidAmount},
		{"abc", ErrInvalidAmount},
		{"1.1234567890", ErrInvalidAmount},
		{"1..2", ErrInvalidAmount},
		{"1,2", ErrInvalidAmount},
		{"9223372037", ErrOverflow},
	}
	for _, tc := range cases {
		_, err := ParseAmount(tc.in)
		if !errors.Is(err, tc.wantErr) {
			t.Fatalf("%s: got %v want %v", tc.in, err, tc.wantErr)
		}
	}
}

func TestAmountString(t *testing.T) {
	cases := []struct {
		in   Amount
		want string
	}{
		{0, "0"},
		{Dollar, "1"},
		{Dollar + 250*MilliDollar, "1.25"},
		{2500, "0.0000025"},
		{-1, "-0.000000001"},
		{-(Dollar + 1), "-1.000000001"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Fatalf("%d: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestAmountAddSubMulDiv(t *testing.T) {
	a := MustParseAmount("10")
	b := MustParseAmount("2.5")

	sum, err := a.Add(b)
	if err != nil || sum.String() != "12.5" {
		t.Fatalf("sum=%s err=%v", sum, err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.String() != "7.5" {
		t.Fatalf("diff=%s err=%v", diff, err)
	}
	product, err := b.Mul(3)
	if err != nil || product.String() != "7.5" {
		t.Fatalf("product=%s err=%v", product, err)
	}
	down, err := MustParseAmount("1").Div(3, RoundDown)
	if err != nil || down.String() != "0.333333333" {
		t.Fatalf("down=%s err=%v", down, err)
	}
	up, err := MustParseAmount("1").Div(3, RoundUp)
	if err != nil || up.String() != "0.333333334" {
		t.Fatalf("up=%s err=%v", up, err)
	}
	halfUp, err := Amount(1).Div(2, RoundHalfUp)
	if err != nil || halfUp != 1 {
		t.Fatalf("halfUp=%d err=%v", halfUp, err)
	}
}

func TestAmountArithmeticErrors(t *testing.T) {
	if _, err := Amount(math.MaxInt64).Add(1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("add overflow: %v", err)
	}
	if _, err := Amount(math.MinInt64).Sub(1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("sub overflow: %v", err)
	}
	if _, err := Amount(math.MaxInt64).Mul(2); !errors.Is(err, ErrOverflow) {
		t.Fatalf("mul overflow: %v", err)
	}
	if _, err := Amount(1).Div(0, RoundDown); !errors.Is(err, ErrDivisionByZero) {
		t.Fatalf("div zero: %v", err)
	}
	if _, err := Amount(1).MulRatio(1, 0, RoundHalfUp); !errors.Is(err, ErrDivisionByZero) {
		t.Fatalf("ratio zero: %v", err)
	}
}

func TestAmountMulRatio(t *testing.T) {
	price := MustParseDollars("2.5")
	got, err := price.MulRatio(1_000_000, 1_000_000, RoundHalfUp)
	if err != nil {
		t.Fatal(err)
	}
	if got != price {
		t.Fatalf("got %s want %s", got, price)
	}

	perMillion := MustParseDollars("5")
	usage, err := perMillion.MulRatio(250_000, 1_000_000, RoundHalfUp)
	if err != nil {
		t.Fatal(err)
	}
	if usage.String() != "1.25" {
		t.Fatalf("got %s want 1.25", usage)
	}
}

func TestAmountJSONAndText(t *testing.T) {
	type payload struct {
		Amount Amount `json:"amount"`
	}
	original := payload{Amount: MustParseAmount("12.34")}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"amount":"12.34"}` {
		t.Fatalf("got %s", data)
	}
	var roundtrip payload
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip.Amount != original.Amount {
		t.Fatalf("got %s want %s", roundtrip.Amount, original.Amount)
	}

	text, err := original.Amount.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	var parsed Amount
	if err := parsed.UnmarshalText(text); err != nil {
		t.Fatal(err)
	}
	if parsed != original.Amount {
		t.Fatalf("got %s want %s", parsed, original.Amount)
	}
}

func TestAmountUnmarshalJSONErrors(t *testing.T) {
	var a Amount
	if err := json.Unmarshal([]byte(`"bad"`), &a); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("got %v", err)
	}
	if err := json.Unmarshal([]byte(`[]`), &a); err == nil {
		t.Fatal("expected error")
	}
}

func TestMoneyConstructionAndFormatting(t *testing.T) {
	m := MustMoney(MustParseAmount("12.34"), USD)
	if got := m.String(); got != "$12.34" {
		t.Fatalf("got %q", got)
	}

	toman := MustMoney(MustParseAmount("125000"), IRT)
	if got := toman.String(); got != "125,000 T" {
		t.Fatalf("got %q", got)
	}
}

func TestParseMoney(t *testing.T) {
	m, err := Parse("1234.567", IRT)
	if err != nil {
		t.Fatal(err)
	}
	if m.Amount != MustParseAmount("1234.567") {
		t.Fatalf("got %s want 1234.567", m.Amount)
	}
	if m.String() != "1,234.567 T" {
		t.Fatalf("got %q", m.String())
	}
}

func TestMoneyOperations(t *testing.T) {
	a := MustMoney(MustParseAmount("1.5"), USD)
	b := MustMoney(MustParseAmount("2.25"), USD)
	sum, err := a.Add(b)
	if err != nil || sum.Amount.String() != "3.75" {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	diff, err := b.Sub(a)
	if err != nil || diff.Amount.String() != "0.75" {
		t.Fatalf("diff=%+v err=%v", diff, err)
	}
	if _, err := a.Add(MustMoney(MustParseAmount("1"), IRT)); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestMoneyJSON(t *testing.T) {
	m := MustMoney(MustParseAmount("12.34"), USD)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip Money
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip.Currency != USD || roundtrip.Amount != MustParseAmount("12.34") {
		t.Fatalf("got %+v", roundtrip)
	}
}

func TestMoneyJSONErrors(t *testing.T) {
	var m Money
	if err := json.Unmarshal([]byte(`{"amount":"1","currency":"BAD"}`), &m); !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("got %v", err)
	}
}
