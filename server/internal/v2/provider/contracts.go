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
	Revision                    int64 `json:"revision"`
	InputNanosPerMillionTokens  int64 `json:"input_nanos_per_million_tokens"`
	OutputNanosPerMillionTokens int64 `json:"output_nanos_per_million_tokens"`
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

// CalculateCost returns the ceiling of token rates expressed in nano-USD per
// million tokens. Overflow is rejected instead of wrapping a budget value.
func CalculateCost(price Price, inputTokens, outputTokens int64) (int64, bool) {
	if inputTokens < 0 || outputTokens < 0 || price.InputNanosPerMillionTokens < 0 || price.OutputNanosPerMillionTokens < 0 {
		return 0, false
	}
	total := new(big.Int).Mul(big.NewInt(inputTokens), big.NewInt(price.InputNanosPerMillionTokens))
	total.Add(total, new(big.Int).Mul(big.NewInt(outputTokens), big.NewInt(price.OutputNanosPerMillionTokens)))
	// Ceiling division ensures a reservation is never rounded below the amount
	// represented by the configured price.
	total.Add(total, big.NewInt(999_999))
	total.Quo(total, big.NewInt(1_000_000))
	if total.Sign() < 0 || !total.IsInt64() {
		return 0, false
	}
	return total.Int64(), true
}
