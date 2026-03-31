package timex

import (
	"testing"
	"time"
)

func TestIsZero(t *testing.T) {
	cases := []struct {
		name string
		ms   MS
		want bool
	}{
		{"zero value is zero", MS(0), true},
		{"one ms is not zero", MS(1), false},
		{"negative is not zero", MS(-1), false},
		{"large value is not zero", MS(1_000_000_000_000), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.ms.IsZero(); got != c.want {
				t.Errorf("MS(%d).IsZero() = %v, want %v", c.ms, got, c.want)
			}
		})
	}
}

func TestZeroString(t *testing.T) {
	if got := MS(0).String(); got != "" {
		t.Errorf("MS(0).String() = %q, want empty", got)
	}
}

func TestNow(t *testing.T) {
	before := time.Now().UnixMilli()
	m := Now()
	after := time.Now().UnixMilli()

	if int64(m) < before || int64(m) > after {
		t.Errorf("Now() = %d, not in [%d, %d]", m, before, after)
	}
}

func TestNowNeverZero(t *testing.T) {
	if Now().IsZero() {
		t.Error("Now() must never return zero")
	}
}

func TestNowMonotonicity(t *testing.T) {
	a := Now()
	b := Now()
	if b < a {
		t.Errorf("second Now() (%d) < first Now() (%d)", b, a)
	}
}

func TestFromTimeRoundtripUTC(t *testing.T) {
	t0 := time.Date(2026, 3, 30, 14, 30, 45, 123_000_000, time.UTC)
	m := From(t0)
	got := m.Time()

	if !got.Equal(t0) {
		t.Errorf("roundtrip: got %v, want %v", got, t0)
	}
	if got.Location() != time.UTC {
		t.Errorf("Time() location = %v, want UTC", got.Location())
	}
}

func TestFromNonUTCTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("timezone America/New_York not available")
	}
	t0 := time.Date(2026, 6, 15, 10, 0, 0, 0, loc)
	m := From(t0)
	got := m.Time()

	// Result must be UTC
	if got.Location() != time.UTC {
		t.Errorf("Time() location = %v, want UTC", got.Location())
	}
	// Must represent the same instant
	if !got.Equal(t0) {
		t.Errorf("non-UTC roundtrip: got %v, want %v (same instant)", got, t0)
	}
}

func TestFromSubMillisecondTruncation(t *testing.T) {
	// Microsecond and nanosecond precision is lost — this is by design
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 999_999, time.UTC) // 999µs 999ns
	m := From(t0)
	got := m.Time()

	// The sub-millisecond portion should be truncated
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("sub-ms truncation: got %v, want %v", got, expected)
	}
}

func TestStringKnownTimestamp(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	m := From(t0)
	s := m.String()

	if s == "" {
		t.Fatal("non-zero MS.String() should not be empty")
	}

	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("String() returned unparseable value %q: %v", s, err)
	}
	if !parsed.Equal(t0) {
		t.Errorf("String() roundtrip: got %v, want %v", parsed, t0)
	}
}

func TestStringWithMilliseconds(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 8, 30, 0, 500_000_000, time.UTC) // .5s
	m := From(t0)
	s := m.String()

	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !parsed.Equal(t0) {
		t.Errorf("roundtrip with millis: got %v, want %v", parsed, t0)
	}
}

func TestSincePast(t *testing.T) {
	past := Now()
	time.Sleep(5 * time.Millisecond)
	elapsed := Since(past)
	if elapsed < 1 {
		t.Errorf("Since(past) = %d, want > 0", elapsed)
	}
}

func TestSinceFuture(t *testing.T) {
	// Since a future timestamp should be negative
	future := Now() + 10_000 // 10 seconds from now
	elapsed := Since(future)
	if elapsed >= 0 {
		t.Errorf("Since(future) = %d, want negative", elapsed)
	}
}

func TestMSArithmetic(t *testing.T) {
	a := MS(1_000)
	b := MS(1_500)

	diff := int64(b - a)
	if diff != 500 {
		t.Errorf("b - a = %d, want 500", diff)
	}
}

func TestMSOrdering(t *testing.T) {
	a := MS(1000)
	b := MS(2000)
	if !(a < b) {
		t.Error("MS(1000) should be < MS(2000)")
	}
	if !(b > a) {
		t.Error("MS(2000) should be > MS(1000)")
	}
	if a == b {
		t.Error("MS(1000) should not equal MS(2000)")
	}
	c := MS(1000)
	if a != c {
		t.Error("MS(1000) should equal MS(1000)")
	}
}

func TestEpochBoundary(t *testing.T) {
	// MS(1) is 1ms after Unix epoch — valid, not zero
	m := MS(1)
	if m.IsZero() {
		t.Error("MS(1) should not be zero")
	}
	got := m.Time()
	want := time.Date(1970, 1, 1, 0, 0, 0, 1_000_000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("MS(1).Time() = %v, want %v", got, want)
	}
}

func TestLargeTimestampYear2100(t *testing.T) {
	t0 := time.Date(2100, 12, 31, 23, 59, 59, 0, time.UTC)
	m := From(t0)
	got := m.Time()

	if !got.Equal(t0) {
		t.Errorf("year 2100 roundtrip: got %v, want %v", got, t0)
	}

	// Verify the underlying int64 is positive and reasonable
	if int64(m) <= 0 {
		t.Errorf("year 2100 MS = %d, should be positive", m)
	}
}

func TestLargeTimestampYear3000(t *testing.T) {
	// Verify no overflow for far-future dates
	t0 := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
	m := From(t0)
	got := m.Time()

	if !got.Equal(t0) {
		t.Errorf("year 3000 roundtrip: got %v, want %v", got, t0)
	}
	if int64(m) <= 0 {
		t.Errorf("year 3000 MS should be positive, got %d", m)
	}
}
