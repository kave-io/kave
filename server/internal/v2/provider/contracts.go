// Package provider defines the narrow data-plane boundary between the V2 HTTP
// gateway and its transactional persistence implementation. It contains no
// HTTP or database types so the gateway and Postgres adapter remain decoupled.
package provider

import (
	"context"
	"errors"
	"math/big"
	"time"

	corev2 "github.com/kave-io/kave/core/v2"
)

var (
	ErrAlreadyInvoked         = errors.New("kave v2: invocation key was already used")
	ErrInvocationInProgress   = errors.New("kave v2: invocation is already in progress")
	ErrReservationUnavailable = errors.New("kave v2: safe usage reservation is unavailable")
	ErrRouteUnavailable       = errors.New("kave v2: provider route is unavailable")
	ErrUnsupportedEndpoint    = errors.New("kave v2: endpoint is not allowed for this agent")
)

const (
	EndpointChatCompletions = "chat/completions"
	EndpointResponses       = "responses"
	EndpointEmbeddings      = "embeddings"
)

// BeginRequest contains only admission metadata and a digest of the provider
// payload. The prompt body itself never crosses the persistence boundary.
type BeginRequest struct {
	Caller           corev2.Caller
	Agent            corev2.Ref
	Endpoint         string
	Scope            corev2.Scope
	InvocationKey    corev2.Ref
	RequestHash      [32]byte
	RequestedModel   string
	InputUpperBound  int64
	InputBounded     bool
	OutputUpperBound int64
	OutputBounded    bool
}

type Price struct {
	Revision                        int64 `json:"revision"`
	InputNanosPerMillionTokens      int64 `json:"input_nanos_per_million_tokens"`
	OutputNanosPerMillionTokens     int64 `json:"output_nanos_per_million_tokens"`
	CacheReadNanosPerMillionTokens  int64 `json:"cache_read_nanos_per_million_tokens"`
	CacheWriteNanosPerMillionTokens int64 `json:"cache_write_nanos_per_million_tokens"`
	ReasoningNanosPerMillionTokens  int64 `json:"reasoning_nanos_per_million_tokens"`
}

// Grant is the short-lived routing result returned after atomic admission.
// Credential is plaintext only in process and must be cleared by the caller.
type Grant struct {
	InvocationID string
	AttemptNo    int
	AccountID    corev2.Ref
	NamespaceID  corev2.Ref
	ServiceKeyID corev2.Ref
	AgentID      string
	RouteID      string
	Provider     string
	Protocol     string
	BaseURL      string
	Model        string
	Credential   []byte
	Price        *Price
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostNanos        int64
	Currency         string
	Model            string
	Reported         bool
}

type AttemptRequest struct {
	Grant     Grant
	AttemptNo int
	StartedAt time.Time
}

type CompleteRequest struct {
	Grant             Grant
	AttemptNo         int
	HTTPStatus        int
	Latency           time.Duration
	ProviderRequestID string
	Usage             Usage
	DeliveryStarted   bool
	// Uncertain means the request may have reached the provider but complete
	// usage could not be observed. Matching reservations are charged at their
	// conservative maxima instead of being released.
	Uncertain  bool
	FinishedAt time.Time
}

type Store interface {
	Begin(context.Context, BeginRequest) (Grant, error)
	StartAttempt(context.Context, AttemptRequest) error
	RenewLease(context.Context, Grant) error
	Complete(context.Context, CompleteRequest) error
}

// CalculateUsageCost returns the ceiling of token rates expressed in nano-USD
// per million tokens. Provider totals include cache reads within input tokens
// and reasoning within output tokens, so those subsets replace (rather than
// add to) their normal rates. Cache writes are a separately reported billable
// dimension. Overflow and structurally impossible usage are rejected.
func CalculateUsageCost(price Price, usage Usage) (int64, bool) {
	if !validPrice(price) || usage.InputTokens < 0 || usage.OutputTokens < 0 ||
		usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 ||
		usage.CacheReadTokens > usage.InputTokens || usage.CacheWriteTokens > usage.InputTokens ||
		usage.ReasoningTokens > usage.OutputTokens {
		return 0, false
	}
	cacheReadRate, cacheWriteRate, reasoningRate := effectiveDetailedRates(price)
	total := multiply(usage.InputTokens-usage.CacheReadTokens, price.InputNanosPerMillionTokens)
	total.Add(total, multiply(usage.CacheReadTokens, cacheReadRate))
	total.Add(total, multiply(usage.CacheWriteTokens, cacheWriteRate))
	total.Add(total, multiply(usage.OutputTokens-usage.ReasoningTokens, price.OutputNanosPerMillionTokens))
	total.Add(total, multiply(usage.ReasoningTokens, reasoningRate))
	// Ceiling division ensures a reservation is never rounded below the amount
	// represented by the configured price.
	total.Add(total, big.NewInt(999_999))
	total.Quo(total, big.NewInt(1_000_000))
	if total.Sign() < 0 || !total.IsInt64() {
		return 0, false
	}
	return total.Int64(), true
}

// CalculateMaximumCost prices bounded input/output using the most expensive
// possible classification of each token. Cache writes can be an additional
// input-side charge, so their effective rate is reserved as well. This can
// over-reserve, but cannot silently under-reserve a cost limit before provider
// usage is known.
func CalculateMaximumCost(price Price, inputUpperBound, outputUpperBound int64) (int64, bool) {
	if !validPrice(price) || inputUpperBound < 0 || outputUpperBound < 0 {
		return 0, false
	}
	cacheReadRate, cacheWriteRate, reasoningRate := effectiveDetailedRates(price)
	inputRate := max(price.InputNanosPerMillionTokens, cacheReadRate)
	inputRateBig := new(big.Int).Add(big.NewInt(inputRate), big.NewInt(cacheWriteRate))
	outputRate := max(price.OutputNanosPerMillionTokens, reasoningRate)
	total := new(big.Int).Mul(big.NewInt(inputUpperBound), inputRateBig)
	total.Add(total, multiply(outputUpperBound, outputRate))
	total.Add(total, big.NewInt(999_999))
	total.Quo(total, big.NewInt(1_000_000))
	if total.Sign() < 0 || !total.IsInt64() {
		return 0, false
	}
	return total.Int64(), true
}

func validPrice(price Price) bool {
	return price.InputNanosPerMillionTokens >= 0 && price.OutputNanosPerMillionTokens >= 0 &&
		price.CacheReadNanosPerMillionTokens >= 0 && price.CacheWriteNanosPerMillionTokens >= 0 &&
		price.ReasoningNanosPerMillionTokens >= 0
}

func effectiveDetailedRates(price Price) (cacheRead, cacheWrite, reasoning int64) {
	cacheRead = price.CacheReadNanosPerMillionTokens
	if cacheRead == 0 {
		cacheRead = price.InputNanosPerMillionTokens
	}
	cacheWrite = price.CacheWriteNanosPerMillionTokens
	if cacheWrite == 0 {
		cacheWrite = price.InputNanosPerMillionTokens
	}
	reasoning = price.ReasoningNanosPerMillionTokens
	if reasoning == 0 {
		reasoning = price.OutputNanosPerMillionTokens
	}
	return cacheRead, cacheWrite, reasoning
}

func multiply(quantity, rate int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(quantity), big.NewInt(rate))
}
