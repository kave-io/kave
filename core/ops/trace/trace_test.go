package trace

import (
	"encoding/json"
	"testing"

	"github.com/kave-io/kave/core/pkg/timex"
)

func TestSpan_ComputeDuration(t *testing.T) {
	tests := []struct {
		name      string
		startedAt timex.MS
		endedAt   timex.MS
		want      int64
	}{
		{"normal", timex.MS(1000), timex.MS(1500), 500},
		{"zero start", timex.MS(0), timex.MS(1500), 0},
		{"zero end", timex.MS(1000), timex.MS(0), 0},
		{"both zero", timex.MS(0), timex.MS(0), 0},
		{"same time", timex.MS(1000), timex.MS(1000), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Span{StartedAt: tt.startedAt, EndedAt: tt.endedAt}
			got := s.ComputeDuration()
			if got != tt.want {
				t.Errorf("ComputeDuration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSourceConstants(t *testing.T) {
	tests := []struct {
		got  SpanSource
		want SpanSource
	}{
		{SourceIntercept, "intercept"},
		{SourceReport, "report"},
		{SourceOTELImport, "otel_import"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("source constant %q != %q", tt.got, tt.want)
		}
	}
}

func TestKindConstants(t *testing.T) {
	tests := []struct {
		got  SpanKind
		want SpanKind
	}{
		{SpanKindAction, "action"},
		{SpanKindObservedAction, "observed_action"},
		{SpanKindImport, "import"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("kind constant %q != %q", tt.got, tt.want)
		}
	}
}

func TestAttrVal_MarshalUnmarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		av   AttrVal
	}{
		{
			name: "string",
			av:   StringAttr("hello world"),
		},
		{
			name: "int",
			av:   IntAttr(42),
		},
		{
			name: "int negative",
			av:   IntAttr(-1000),
		},
		{
			name: "float",
			av:   FloatAttr(3.14159),
		},
		{
			name: "float negative",
			av:   FloatAttr(-2.71828),
		},
		{
			name: "bool true",
			av:   BoolAttr(true),
		},
		{
			name: "bool false",
			av:   BoolAttr(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.av)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got AttrVal
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Verify type and value match
			switch {
			case tt.av.IsString():
				if !got.IsString() || got.Str() != tt.av.Str() {
					t.Errorf("string mismatch: got %q, want %q", got.Str(), tt.av.Str())
				}
			case tt.av.IsInt():
				if !got.IsInt() || got.Int() != tt.av.Int() {
					t.Errorf("int mismatch: got %d, want %d", got.Int(), tt.av.Int())
				}
			case tt.av.IsFloat():
				if !got.IsFloat() || got.Float() != tt.av.Float() {
					t.Errorf("float mismatch: got %f, want %f", got.Float(), tt.av.Float())
				}
			case tt.av.IsBool():
				if !got.IsBool() || got.Bool() != tt.av.Bool() {
					t.Errorf("bool mismatch: got %v, want %v", got.Bool(), tt.av.Bool())
				}
			}
		})
	}
}

func TestAttrVal_UnmarshalBadInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{
			name:      "empty object",
			input:     "{}",
			shouldErr: false, // silently produces zero value
		},
		{
			name:      "missing value field",
			input:     `{"type":"string"}`,
			shouldErr: false, // silently produces zero value
		},
		{
			name:      "unknown type",
			input:     `{"type":"unknown","value":1}`,
			shouldErr: false, // silently produces zero value
		},
		{
			name:      "null input",
			input:     "null",
			shouldErr: false, // UnmarshalJSON doesn't panic; type check fails gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var av AttrVal
			err := json.Unmarshal([]byte(tt.input), &av)
			if tt.shouldErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// For bad inputs, we expect a zero-valued AttrVal (or silently ignored)
			// The implementation should not panic or leave corrupt state.
		})
	}
}

func TestAttrVal_AccessorMethods(t *testing.T) {
	tests := []struct {
		name          string
		av            AttrVal
		expectedType  string
		expectedValue interface{}
	}{
		{
			name:          "string accessor",
			av:            StringAttr("test"),
			expectedType:  "string",
			expectedValue: "test",
		},
		{
			name:          "int accessor",
			av:            IntAttr(123),
			expectedType:  "int",
			expectedValue: int64(123),
		},
		{
			name:          "float accessor",
			av:            FloatAttr(1.5),
			expectedType:  "float",
			expectedValue: 1.5,
		},
		{
			name:          "bool accessor",
			av:            BoolAttr(true),
			expectedType:  "bool",
			expectedValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.expectedType {
			case "string":
				if !tt.av.IsString() {
					t.Errorf("IsString() = false, want true")
				}
				if got := tt.av.Str(); got != tt.expectedValue {
					t.Errorf("Str() = %q, want %q", got, tt.expectedValue)
				}
				if tt.av.IsInt() || tt.av.IsFloat() || tt.av.IsBool() {
					t.Error("other type checks should be false")
				}

			case "int":
				if !tt.av.IsInt() {
					t.Errorf("IsInt() = false, want true")
				}
				if got := tt.av.Int(); got != tt.expectedValue {
					t.Errorf("Int() = %d, want %d", got, tt.expectedValue)
				}
				if tt.av.IsString() || tt.av.IsFloat() || tt.av.IsBool() {
					t.Error("other type checks should be false")
				}

			case "float":
				if !tt.av.IsFloat() {
					t.Errorf("IsFloat() = false, want true")
				}
				if got := tt.av.Float(); got != tt.expectedValue {
					t.Errorf("Float() = %f, want %f", got, tt.expectedValue)
				}
				if tt.av.IsString() || tt.av.IsInt() || tt.av.IsBool() {
					t.Error("other type checks should be false")
				}

			case "bool":
				if !tt.av.IsBool() {
					t.Errorf("IsBool() = false, want true")
				}
				if got := tt.av.Bool(); got != tt.expectedValue {
					t.Errorf("Bool() = %v, want %v", got, tt.expectedValue)
				}
				if tt.av.IsString() || tt.av.IsInt() || tt.av.IsFloat() {
					t.Error("other type checks should be false")
				}
			}
		})
	}
}
