package postgres

import (
	"bytes"
	"context"
	"errors"
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
	if err := assignProviderReservations(cost, request, price); err != nil || cost[0].reserve != 280 {
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
			InputTokens: 12, OutputTokens: 3, CacheReadTokens: 4,
			ReasoningTokens: 2, CostNanos: 42, Currency: "USD",
		},
	}
	accounted := accountingFromCompletion(req)
	accounted.include(corev2.MetricInputTokens, 100)
	accounted.include(corev2.MetricOutputTokens, 2)
	accounted.include(corev2.MetricCostNanoUSD, 500)
	accounted.include(corev2.MetricRequests, 1)
	if accounted.RequestCount != 1 || accounted.InputTokens != 100 || accounted.OutputTokens != 3 ||
		accounted.CacheReadTokens != 4 || accounted.ReasoningTokens != 2 ||
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
