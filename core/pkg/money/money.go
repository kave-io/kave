package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Amount stores a monetary value in nano-units of the major currency.
// The zero value is valid and exact.
type Amount int64

const (
	NanoDollar  Amount = 1
	MicroDollar Amount = 1_000
	MilliDollar Amount = 1_000_000
	Dollar      Amount = 1_000_000_000
)

type CurrencyCode string

const (
	USD CurrencyCode = "USD"
	EUR CurrencyCode = "EUR"
	GBP CurrencyCode = "GBP"
	CHF CurrencyCode = "CHF"
	IRR CurrencyCode = "IRR"
	IRT CurrencyCode = "IRT"
)

type Currency struct {
	Code           CurrencyCode
	Name           string
	Symbol         string
	SymbolOnLeft   bool
	SpaceBetween   bool
	DecimalSep     string
	ThousandsSep   string
	FractionDigits int
}

type Money struct {
	Amount   Amount       `json:"amount"`
	Currency CurrencyCode `json:"currency"`
}

type RoundingMode int

const (
	RoundDown RoundingMode = iota
	RoundUp
	RoundHalfUp
)

var (
	ErrInvalidAmount    = errors.New("money: invalid amount")
	ErrOverflow         = errors.New("money: overflow")
	ErrDivisionByZero   = errors.New("money: division by zero")
	ErrUnknownCurrency  = errors.New("money: unknown currency")
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
)

var currencies = map[CurrencyCode]Currency{
	USD: {Code: USD, Name: "US Dollar", Symbol: "$", SymbolOnLeft: true, DecimalSep: ".", ThousandsSep: ",", FractionDigits: 2},
	EUR: {Code: EUR, Name: "Euro", Symbol: "EUR", SymbolOnLeft: false, SpaceBetween: true, DecimalSep: ",", ThousandsSep: ".", FractionDigits: 2},
	GBP: {Code: GBP, Name: "Pound Sterling", Symbol: "GBP", SymbolOnLeft: true, SpaceBetween: true, DecimalSep: ".", ThousandsSep: ",", FractionDigits: 2},
	CHF: {Code: CHF, Name: "Swiss Franc", Symbol: "CHF", SymbolOnLeft: false, SpaceBetween: true, DecimalSep: ".", ThousandsSep: "'", FractionDigits: 2},
	IRR: {Code: IRR, Name: "Iranian Rial", Symbol: "IRR", SymbolOnLeft: false, SpaceBetween: true, DecimalSep: ".", ThousandsSep: ",", FractionDigits: 0},
	IRT: {Code: IRT, Name: "Iranian Toman", Symbol: "IRT", SymbolOnLeft: false, SpaceBetween: true, DecimalSep: ".", ThousandsSep: ",", FractionDigits: 0},
}

func LookupCurrency(code string) (Currency, bool) {
	c, ok := currencies[CurrencyCode(strings.ToUpper(strings.TrimSpace(code)))]
	return c, ok
}

func MustCurrency(code string) Currency {
	c, ok := LookupCurrency(code)
	if !ok {
		panic(fmt.Sprintf("money: unknown currency %q", code))
	}
	return c
}

func ParseAmount(s string) (Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidAmount
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, ErrInvalidAmount
	}
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	if intPart == "" || !digitsOnly(intPart) {
		return 0, ErrInvalidAmount
	}
	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
		if !digitsOnly(fracPart) || len(fracPart) > 9 {
			return 0, ErrInvalidAmount
		}
	}
	if intVal > math.MaxInt64/int64(Dollar) {
		return 0, ErrOverflow
	}
	total := intVal * int64(Dollar)
	if fracPart != "" {
		fracPart += strings.Repeat("0", 9-len(fracPart))
		fracVal, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, ErrInvalidAmount
		}
		total += fracVal
	}
	if neg {
		total = -total
	}
	return Amount(total), nil
}

func MustParseAmount(s string) Amount {
	a, err := ParseAmount(s)
	if err != nil {
		panic(err)
	}
	return a
}

func MustParseDollars(s string) Amount { return MustParseAmount(s) }

func (a Amount) Nano() int64 { return int64(a) }

func (a Amount) IsZero() bool { return a == 0 }

func (a Amount) Abs() Amount {
	if a < 0 {
		return -a
	}
	return a
}

func (a Amount) Add(other Amount) (Amount, error) {
	out := int64(a) + int64(other)
	if (a > 0 && other > 0 && Amount(out) < 0) || (a < 0 && other < 0 && Amount(out) > 0) {
		return 0, ErrOverflow
	}
	return Amount(out), nil
}

func (a Amount) Sub(other Amount) (Amount, error) {
	return a.Add(-other)
}

func (a Amount) Mul(n int64) (Amount, error) {
	if a == 0 || n == 0 {
		return 0, nil
	}
	if int64(a) == math.MinInt64 && n == -1 {
		return 0, ErrOverflow
	}
	if n > 0 && int64(a) > math.MaxInt64/n || n < 0 && int64(a) < math.MinInt64/n {
		return 0, ErrOverflow
	}
	return Amount(int64(a) * n), nil
}

func (a Amount) Div(n int64, mode RoundingMode) (Amount, error) {
	if n == 0 {
		return 0, ErrDivisionByZero
	}
	return divRound(int64(a), n, mode)
}

func (a Amount) MulRatio(numerator, denominator int64, mode RoundingMode) (Amount, error) {
	if denominator == 0 {
		return 0, ErrDivisionByZero
	}
	rat := new(big.Rat).SetFrac64(int64(a), 1)
	rat.Mul(rat, new(big.Rat).SetFrac64(numerator, denominator))
	return ratToAmount(rat, mode)
}

func (a Amount) String() string {
	if a == 0 {
		return "0"
	}
	neg := a < 0
	raw := int64(a)
	if neg {
		raw = -raw
	}
	major := raw / int64(Dollar)
	frac := raw % int64(Dollar)
	if frac == 0 {
		if neg {
			return "-" + strconv.FormatInt(major, 10)
		}
		return strconv.FormatInt(major, 10)
	}
	out := strconv.FormatInt(major, 10) + "." + strings.TrimRight(fmt.Sprintf("%09d", frac), "0")
	if neg {
		return "-" + out
	}
	return out
}

func (a Amount) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*a = 0
		return nil
	}
	if len(data) == 0 {
		return ErrInvalidAmount
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v, err := ParseAmount(s)
		if err != nil {
			return err
		}
		*a = v
		return nil
	}
	v, err := ParseAmount(string(data))
	if err != nil {
		return err
	}
	*a = v
	return nil
}

func (a Amount) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

func (a *Amount) UnmarshalText(data []byte) error {
	v, err := ParseAmount(string(data))
	if err != nil {
		return err
	}
	*a = v
	return nil
}

func NewMoney(amount Amount, currency CurrencyCode) (Money, error) {
	if _, ok := currencies[currency]; !ok {
		return Money{}, ErrUnknownCurrency
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func MustMoney(amount Amount, currency CurrencyCode) Money {
	m, err := NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func Parse(s string, currency CurrencyCode) (Money, error) {
	if _, ok := currencies[currency]; !ok {
		return Money{}, ErrUnknownCurrency
	}
	amount, err := ParseAmount(normalizeDecimal(s, currency))
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func MustParse(s string, currency CurrencyCode) Money {
	m, err := Parse(s, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) CurrencyMeta() Currency {
	if c, ok := currencies[m.Currency]; ok {
		return c
	}
	return currencies[USD]
}

func (m Money) String() string {
	cur := m.CurrencyMeta()
	body := formatAmountForCurrency(m.Amount, cur, cur.FractionDigits)
	if cur.SymbolOnLeft {
		if cur.SpaceBetween {
			return cur.Symbol + " " + body
		}
		return cur.Symbol + body
	}
	if cur.SpaceBetween {
		return body + " " + cur.Symbol
	}
	return body + cur.Symbol
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	sum, err := m.Amount.Add(other.Amount)
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: sum, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	diff, err := m.Amount.Sub(other.Amount)
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: diff, Currency: m.Currency}, nil
}

type moneyJSON struct {
	Amount   Amount       `json:"amount"`
	Currency CurrencyCode `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(moneyJSON(m))
}

func (m *Money) UnmarshalJSON(data []byte) error {
	var p moneyJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if _, ok := currencies[p.Currency]; !ok {
		return ErrUnknownCurrency
	}
	*m = Money(p)
	return nil
}

func normalizeDecimal(s string, currency CurrencyCode) string {
	s = strings.TrimSpace(s)
	cur, ok := currencies[currency]
	if ok && cur.DecimalSep != "." {
		s = strings.ReplaceAll(s, cur.DecimalSep, ".")
	}
	return s
}

func formatAmountForCurrency(a Amount, cur Currency, fractionDigits int) string {
	plain := a.String()
	neg := strings.HasPrefix(plain, "-")
	if neg {
		plain = plain[1:]
	}
	parts := strings.SplitN(plain, ".", 2)
	intPart := formatThousands(parts[0], cur.ThousandsSep)
	out := intPart
	if len(parts) == 2 {
		frac := parts[1]
		if fractionDigits >= 0 && len(frac) > fractionDigits {
			frac = strings.TrimRight(frac[:fractionDigits], "0")
		}
		if frac != "" {
			out += cur.DecimalSep + frac
		}
	}
	if neg {
		return "-" + out
	}
	return out
}

func formatThousands(s, sep string) string {
	if sep == "" || len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func digitsOnly(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func divRound(numerator, denominator int64, mode RoundingMode) (Amount, error) {
	if denominator == 0 {
		return 0, ErrDivisionByZero
	}
	// Negating math.MinInt64 overflows int64; escalate to big.Int for safety.
	if numerator == math.MinInt64 || denominator == math.MinInt64 {
		r := new(big.Rat).SetFrac(big.NewInt(numerator), big.NewInt(denominator))
		return ratToAmount(r, mode)
	}
	neg := (numerator < 0) != (denominator < 0)
	if numerator < 0 {
		numerator = -numerator
	}
	if denominator < 0 {
		denominator = -denominator
	}
	quotient := numerator / denominator
	remainder := numerator % denominator
	switch mode {
	case RoundDown:
	case RoundUp:
		if remainder != 0 {
			if quotient == math.MaxInt64 {
				return 0, ErrOverflow
			}
			quotient++
		}
	case RoundHalfUp:
		if remainder*2 >= denominator {
			if quotient == math.MaxInt64 {
				return 0, ErrOverflow
			}
			quotient++
		}
	default:
		return 0, ErrInvalidAmount
	}
	if neg {
		quotient = -quotient
	}
	return Amount(quotient), nil
}

func ratToAmount(r *big.Rat, mode RoundingMode) (Amount, error) {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	neg := num.Sign() < 0
	if neg {
		num.Neg(num)
	}
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	switch mode {
	case RoundDown:
	case RoundUp:
		if rem.Sign() != 0 {
			q.Add(q, big.NewInt(1))
		}
	case RoundHalfUp:
		doubleRem := new(big.Int).Lsh(rem, 1)
		if doubleRem.Cmp(den) >= 0 {
			q.Add(q, big.NewInt(1))
		}
	default:
		return 0, ErrInvalidAmount
	}
	if neg {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return 0, ErrOverflow
	}
	return Amount(q.Int64()), nil
}
