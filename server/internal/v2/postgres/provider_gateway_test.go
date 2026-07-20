package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

func TestAssignProviderReservationsFailClosedOnlyForMatchingMetrics(t *testing.T) {
	request := provider.BeginRequest{InputUpperBound: 100, InputBounded: true}
	requestOnly := []providerLimit{{metric: corev2.MetricRequests}}
	if err := assignProviderReservations(requestOnly, request, nil); err != nil || requestOnly[0].reserve != 1 {
		t.Fatalf("request reservation = %#v, %v", requestOnly, err)
	}

	output := []providerLimit{{metric: corev2.MetricOutputTokens}}
	if err := assignProviderReservations(output, request, nil); !errors.Is(err, provider.ErrReservationUnavailable) {
		t.Fatalf("unbounded output error = %v", err)
	}
	cost := []providerLimit{{metric: corev2.MetricCostNanoUSD, currency: "USD"}}
	request.OutputBounded, request.OutputUpperBound = true, 20
	if err := assignProviderReservations(cost, request, nil); !errors.Is(err, provider.ErrReservationUnavailable) {
		t.Fatalf("missing price error = %v", err)
	}
	price := &provider.Price{InputNanosPerMillionTokens: 2_000_000, OutputNanosPerMillionTokens: 4_000_000}
	// Before detailed usage is known, the compatibility cache-write rate is an
	// additional input-side maximum: 100*(2+2) + 20*4 = 480.
	if err := assignProviderReservations(cost, request, price); err != nil || cost[0].reserve != 480 {
		t.Fatalf("cost reservation = %#v, %v", cost, err)
	}
}

func TestProviderPriceJSONIsAlwaysAnObject(t *testing.T) {
	if got := string(providerPriceJSON(nil)); got != "{}" {
		t.Fatalf("nil price JSON = %q", got)
	}
}

func TestProviderRouteRequiresVersionedPriceForResolvedModel(t *testing.T) {
	document := routePriceDocument{Models: map[string]routeModelPrice{
		"gpt-safe": {InputNanosPerMillionTokens: 0, OutputNanosPerMillionTokens: 0},
	}}
	if price := priceForModel(1, document, "gpt-safe"); price == nil || price.Revision != 1 {
		t.Fatalf("explicit zero price = %#v, want versioned price", price)
	}
	if price := priceForModel(1, document, "other"); price != nil {
		t.Fatalf("missing-model price = %#v, want nil", price)
	}
	if price := priceForModel(0, document, "gpt-safe"); price != nil {
		t.Fatalf("missing-revision price = %#v, want nil", price)
	}
}

func TestProviderValidationEvidenceIsStrictlyBounded(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		input    corev2.ProviderRouteValidationEvidence
		expected corev2.ProviderRouteValidationEvidence
	}{
		{
			name: "safe", input: corev2.ProviderRouteValidationEvidence{HTTPStatus: 200, ProviderRequestID: "request-123"},
			expected: corev2.ProviderRouteValidationEvidence{HTTPStatus: 200, ProviderRequestID: "request-123"},
		},
		{name: "low status", input: corev2.ProviderRouteValidationEvidence{HTTPStatus: 99}, expected: corev2.ProviderRouteValidationEvidence{}},
		{name: "high status", input: corev2.ProviderRouteValidationEvidence{HTTPStatus: 600}, expected: corev2.ProviderRouteValidationEvidence{}},
		{
			name: "header injection", input: corev2.ProviderRouteValidationEvidence{HTTPStatus: 401, ProviderRequestID: "safe\r\nX-Leak: secret"},
			expected: corev2.ProviderRouteValidationEvidence{HTTPStatus: 401},
		},
		{
			name: "oversized request id", input: corev2.ProviderRouteValidationEvidence{HTTPStatus: 503, ProviderRequestID: strings.Repeat("x", 256)},
			expected: corev2.ProviderRouteValidationEvidence{HTTPStatus: 503},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeProviderValidationEvidence(test.input); got != test.expected {
				t.Fatalf("sanitizeProviderValidationEvidence() = %+v, want %+v", got, test.expected)
			}
		})
	}
}

func TestRouteValidationDocumentBindsEachModelToSecretVersion(t *testing.T) {
	t.Parallel()
	document, err := decodeRouteValidationDocument([]byte(`{
  "models": {
    "gpt-safe": {"secret_version": 3, "validated_at_ms": 10, "http_status": 200},
    "gpt-other": {"secret_version": 3, "validated_at_ms": 20, "http_status": 204, "provider_request_id": "request-2"}
  },
  "last_attempt": {"model": "gpt-other", "secret_version": 3, "attempted_at_ms": 20, "http_status": 204, "validated": true}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if !document.validates("gpt-safe", 3) || !document.validates("gpt-other", 3) ||
		document.validates("gpt-safe", 4) || document.validates("missing", 3) {
		t.Fatalf("validation bindings = %#v", document.Models)
	}
	model, version, validatedAt := document.latest()
	if model != "gpt-other" || version != 3 || validatedAt.UnixMilli() != 20 {
		t.Fatalf("latest = %q/%d/%v", model, version, validatedAt)
	}
}

func TestRouteValidationDocumentRejectsUnboundedOrMalformedEvidence(t *testing.T) {
	t.Parallel()
	tooMany := routeValidationDocument{Models: make(map[string]routeModelValidationEvidence)}
	for i := 0; i <= maxProviderRouteValidationModels; i++ {
		tooMany.Models[fmt.Sprintf("model-%d", i)] = routeModelValidationEvidence{SecretVersion: 1, ValidatedAtMS: 1, HTTPStatus: 200}
	}
	tooManyJSON, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"unknown":true}`),
		[]byte(`{"models":{"bad model":{"secret_version":1,"validated_at_ms":1,"http_status":200}},"last_attempt":{"model":"bad model","secret_version":1,"attempted_at_ms":1,"validated":true}}`),
		[]byte(`{"models":{"model":{"secret_version":1,"validated_at_ms":1,"http_status":401}},"last_attempt":{"model":"model","secret_version":1,"attempted_at_ms":1,"validated":false}}`),
		[]byte(`{"models":{"model":{"secret_version":1,"validated_at_ms":1,"http_status":200,"provider_request_id":"bad\r\nid"}},"last_attempt":{"model":"model","secret_version":1,"attempted_at_ms":1,"validated":true}}`),
		[]byte(`{} {}`),
		tooManyJSON,
	} {
		if _, err := decodeRouteValidationDocument(raw); err == nil {
			t.Fatalf("decodeRouteValidationDocument(%q) succeeded", raw)
		}
	}
}

func TestNormalizeProviderCredentialRejectsHeaderInjection(t *testing.T) {
	for _, invalid := range [][]byte{
		nil, {}, []byte("Bearer "), []byte(" leading"), []byte("trailing "),
		[]byte("secret\r\nX-Evil: yes"), bytes.Repeat([]byte("x"), maxProviderCredentialBytes+1),
		{0xff},
	} {
		if credential, ok := normalizeProviderCredential(invalid); ok || credential != nil {
			t.Fatalf("normalizeProviderCredential(%q) = %q, %v; want rejected", invalid, credential, ok)
		}
	}
	for _, valid := range [][]byte{[]byte("sk-test_123.abc"), []byte("Bearer sk-test_123.abc")} {
		credential, ok := normalizeProviderCredential(valid)
		if !ok || string(credential) != "sk-test_123.abc" {
			t.Fatalf("normalizeProviderCredential(%q) = %q, %v", valid, credential, ok)
		}
		if len(valid) > 0 && len(credential) > 0 && &valid[0] == &credential[0] {
			t.Fatal("normalized credential aliases cipher plaintext")
		}
		clear(credential)
	}
}

func TestUncertainSettlementChargesReservationMaximum(t *testing.T) {
	req := provider.CompleteRequest{Uncertain: true, Usage: provider.Usage{OutputTokens: 80}}
	if got := settlementQuantity(req, reservationRow{metric: corev2.MetricOutputTokens, quantity: 99}); got != 99 {
		t.Fatalf("settled quantity below reservation = %d", got)
	}
	req.Usage.OutputTokens = 120
	if got := settlementQuantity(req, reservationRow{metric: corev2.MetricOutputTokens, quantity: 99}); got != 120 {
		t.Fatalf("settled quantity discarded observed overage = %d", got)
	}
}

func TestCanonicalProviderAccountingIncludesConservativeCharge(t *testing.T) {
	req := provider.CompleteRequest{
		DeliveryStarted: true,
		Usage: provider.Usage{
			InputTokens: 12, OutputTokens: 3, CacheReadTokens: 4, CacheWriteTokens: 2,
			ReasoningTokens: 2, CostNanos: 42, Currency: "USD",
		},
	}
	accounted := accountingFromCompletion(req)
	accounted.include(corev2.MetricInputTokens, 100)
	accounted.include(corev2.MetricOutputTokens, 2)
	accounted.include(corev2.MetricCostNanoUSD, 500)
	accounted.include(corev2.MetricRequests, 1)
	if accounted.RequestCount != 1 || accounted.InputTokens != 100 || accounted.OutputTokens != 3 ||
		accounted.CacheReadTokens != 4 || accounted.CacheWriteTokens != 2 || accounted.ReasoningTokens != 2 ||
		accounted.CostNanos != 500 || accounted.Currency != "USD" || !accounted.Estimated {
		t.Fatalf("accounted usage = %+v", accounted)
	}
}

func TestCanonicalProviderAccountingMarksUnknownUsageEstimatedWithoutLimits(t *testing.T) {
	accounted := accountingFromCompletion(provider.CompleteRequest{
		DeliveryStarted: true,
		Uncertain:       true,
		Usage:           provider.Usage{Reported: false},
	})
	if accounted.RequestCount != 1 || !accounted.Estimated {
		t.Fatalf("accounted usage = %+v, want one estimated request", accounted)
	}
}

func TestProviderCompletionRejectsMalformedUsageBeforeDatabase(t *testing.T) {
	store := &ProviderStore{}
	for _, usage := range []provider.Usage{
		{InputTokens: -1},
		{OutputTokens: -1},
		{CacheReadTokens: -1},
		{CacheWriteTokens: -1},
		{ReasoningTokens: -1},
		{InputTokens: 1, CacheReadTokens: 2},
		{InputTokens: 1, CacheWriteTokens: 2},
		{OutputTokens: 1, ReasoningTokens: 2},
		{CostNanos: -1},
		{Currency: "EUR"},
		{Model: "bad model"},
	} {
		err := store.Complete(context.Background(), provider.CompleteRequest{Usage: usage})
		if !errors.Is(err, corev2.ErrInvalidArgument) {
			t.Fatalf("Complete(%+v) error = %v, want invalid argument", usage, err)
		}
	}
}
