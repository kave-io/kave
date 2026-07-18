// Package gateway implements Kave V2's deliberately small OpenAI-compatible
// data plane. It authenticates a namespace service key, admits the invocation,
// and proxies only three exact provider endpoints.
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	coreopenai "github.com/kave-io/kave/core/connectors/llm/openai"
	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

const (
	HeaderTenant        = "Kave-Tenant"
	HeaderActor         = "Kave-Actor"
	HeaderBillTo        = "Kave-Bill-To"
	HeaderSession       = "Kave-Session"
	HeaderFeature       = "Kave-Feature"
	HeaderInvocationKey = "X-Kave-Invocation-Key"

	maxRequestBody    = 8 << 20
	maxUsageCapture   = 16 << 20
	maxProviderModel  = 160
	settlementTimeout = 10 * time.Second
	leaseRenewEvery   = 4 * time.Minute
	leaseRenewTimeout = 10 * time.Second
)

type Handler struct {
	auth         httpapi.Authenticator
	store        provider.Store
	client       *providerHTTPClient
	logger       *slog.Logger
	now          func() time.Time
	renewEvery   time.Duration
	renewTimeout time.Duration
}

func New(auth httpapi.Authenticator, store provider.Store, policy ProviderEgressPolicy, logger *slog.Logger) (*Handler, error) {
	transport, err := NewProviderTransport(policy)
	if err != nil {
		return nil, err
	}
	return newWithTransport(auth, store, transport, logger)
}

// newWithTransport exists only to keep unit tests deterministic. Production
// assembly calls New, which always installs the restricted provider transport.
func newWithTransport(auth httpapi.Authenticator, store provider.Store, transport http.RoundTripper, logger *slog.Logger) (*Handler, error) {
	if auth == nil || store == nil {
		return nil, errors.New("v2 gateway: authenticator and store are required")
	}
	if transport == nil {
		return nil, errors.New("v2 gateway: provider transport is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		auth:         auth,
		store:        store,
		client:       newProviderHTTPClient(transport),
		logger:       logger,
		now:          time.Now,
		renewEvery:   leaseRenewEvery,
		renewTimeout: leaseRenewTimeout,
	}, nil
}

// Register mounts only the exact, method-qualified routes supported by the V2
// SDK. Go's ServeMux returns 405 for other methods and does not let a wildcard
// suffix accidentally expose another provider endpoint.
func Register(mux *http.ServeMux, handler *Handler) error {
	if mux == nil || handler == nil {
		return errors.New("v2 gateway: HTTP mux and handler are required")
	}
	for _, endpoint := range []string{
		provider.EndpointChatCompletions,
		provider.EndpointResponses,
		provider.EndpointEmbeddings,
	} {
		mux.Handle("POST /v2/agents/{agent}/openai/"+endpoint, handler)
	}
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	caller, status, err := h.authenticate(r)
	if err != nil {
		writeError(w, status, errorCode(status))
		return
	}
	endpoint, ok := endpointFromPath(r.URL.Path)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	agent := corev2.Ref(r.PathValue("agent"))
	if err := agent.ValidateName("agent", true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	scope, invocationKey, err := requestScope(r.Header)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_scope")
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "json_required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	body, document, err := readJSONDocument(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	requestedModel, err := optionalString(document, "model")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_model")
		return
	}
	outputBound, outputBounded, err := outputUpperBound(endpoint, document)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token_limit")
		return
	}
	inputBounded := !containsUnboundedProviderInput(document)
	// Admission reserves before the route's final model is known. Build the
	// exact outbound JSON shape with a maximum-length, JSON-safe model first so
	// whitespace removal, HTML escaping, invalid UTF-8 replacement, and injected
	// stream_options can never make the real upstream body exceed the reserved
	// input bound.
	document["model"] = strings.Repeat("m", maxProviderModel)
	ensureStreamingUsage(endpoint, document)
	boundedBody, err := json.Marshal(document)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	hash := invocationHash(agent, endpoint, scope, invocationKey, body)

	grant, err := h.store.Begin(r.Context(), provider.BeginRequest{
		Caller: caller, Agent: agent, Endpoint: endpoint, Scope: scope,
		InvocationKey: invocationKey, RequestHash: hash,
		RequestedModel: requestedModel, InputUpperBound: int64(len(boundedBody)),
		InputBounded: inputBounded, OutputUpperBound: outputBound,
		OutputBounded: outputBounded,
	})
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	defer clear(grant.Credential)
	w.Header().Set("X-Kave-Invocation-ID", grant.InvocationID)

	document["model"] = grant.Model
	upstreamBody, err := json.Marshal(document)
	if err != nil {
		h.abort(r.Context(), grant, false)
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if len(upstreamBody) > len(boundedBody) {
		h.abort(r.Context(), grant, false)
		h.logger.Error("v2 provider input exceeded preflight bound", "invocation_id", grant.InvocationID)
		writeError(w, http.StatusServiceUnavailable, "accounting_unavailable")
		return
	}
	upstreamURL, err := providerURL(grant.BaseURL, endpoint)
	if err != nil {
		h.abort(r.Context(), grant, false)
		h.logger.Error("v2 provider route rejected at runtime", "invocation_id", grant.InvocationID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "route_unavailable")
		return
	}

	started := h.now().UTC()
	if err := h.store.StartAttempt(r.Context(), provider.AttemptRequest{Grant: grant, AttemptNo: grant.AttemptNo, StartedAt: started}); err != nil {
		h.abort(r.Context(), grant, false)
		h.logger.Error("v2 provider attempt admission failed", "invocation_id", grant.InvocationID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "accounting_unavailable")
		return
	}

	providerCtx, cancelProvider := context.WithCancel(r.Context())
	defer cancelProvider()
	upstream, err := http.NewRequestWithContext(providerCtx, http.MethodPost, upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		h.abort(r.Context(), grant, false)
		writeError(w, http.StatusServiceUnavailable, "route_unavailable")
		return
	}
	upstream.Header = outboundHeaders(r.Header, grant.Credential, invocationKey)
	heartbeatResult := make(chan error, 1)
	go h.heartbeatLease(providerCtx, cancelProvider, grant, heartbeatResult)
	response, err := h.client.Do(upstream)
	if err != nil {
		cancelProvider()
		leaseErr := <-heartbeatResult
		if leaseErr != nil {
			h.logger.Error("v2 provider lease renewal failed", "invocation_id", grant.InvocationID, "error", leaseErr)
		}
		h.complete(r.Context(), provider.CompleteRequest{
			Grant: grant, AttemptNo: grant.AttemptNo, Latency: h.now().Sub(started),
			DeliveryStarted: true, Uncertain: true, FinishedAt: h.now().UTC(),
		})
		writeError(w, http.StatusBadGateway, "provider_unavailable")
		return
	}
	defer response.Body.Close()

	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("X-Kave-Invocation-ID", grant.InvocationID)
	w.WriteHeader(response.StatusCode)
	capture := newTailCapture(maxUsageCapture)
	stream := strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	copyErr := copyProviderBody(w, response.Body, capture, stream)
	cancelProvider()
	leaseErr := <-heartbeatResult
	parsed, parseErr := coreopenai.NewConnector(nil).ParseResponse(capture.Bytes(), response.Header.Get("Content-Type"))
	usage := provider.Usage{Model: grant.Model}
	if parseErr == nil && parsed != nil && parsed.TokenUsage != nil && validTokenUsage(parsed.TokenUsage.InputTokens, parsed.TokenUsage.OutputTokens, parsed.TokenUsage.CacheRead, parsed.TokenUsage.CacheWrite, parsed.TokenUsage.Reasoning) {
		usage.InputTokens = int64(parsed.TokenUsage.InputTokens)
		usage.OutputTokens = int64(parsed.TokenUsage.OutputTokens)
		usage.CacheReadTokens = int64(parsed.TokenUsage.CacheRead)
		usage.CacheWriteTokens = int64(parsed.TokenUsage.CacheWrite)
		usage.ReasoningTokens = int64(parsed.TokenUsage.Reasoning)
		usage.Reported = true
		if validProviderModel(parsed.TokenUsage.Model) {
			usage.Model = parsed.TokenUsage.Model
		}
		if grant.Price != nil {
			if cost, ok := provider.CalculateCost(*grant.Price, usage.InputTokens, usage.OutputTokens); ok {
				usage.CostNanos, usage.Currency = cost, "USD"
			} else {
				usage.Reported = false
			}
		}
	}
	// Provider errors are conservatively treated as potentially billable when
	// no trustworthy usage was returned. If usage was reported, settle that
	// observed amount even for a 4xx/5xx instead of permanently charging the
	// maximum reservation.
	uncertain := copyErr != nil || leaseErr != nil || !usage.Reported
	if leaseErr != nil {
		h.logger.Error("v2 provider lease renewal failed", "invocation_id", grant.InvocationID, "error", leaseErr)
	}
	h.complete(r.Context(), provider.CompleteRequest{
		Grant: grant, AttemptNo: grant.AttemptNo, HTTPStatus: response.StatusCode,
		Latency: h.now().Sub(started), ProviderRequestID: providerRequestID(response.Header),
		Usage: usage, DeliveryStarted: true, Uncertain: uncertain, FinishedAt: h.now().UTC(),
	})
	if copyErr != nil {
		h.logger.Warn("v2 provider response copy failed", "invocation_id", grant.InvocationID, "error", copyErr)
	}
}

func validTokenUsage(values ...int) bool {
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return true
}

func validProviderModel(model string) bool {
	return corev2.Ref(model).Validate("provider_usage.model", true) == nil
}

func (h *Handler) heartbeatLease(ctx context.Context, cancelProvider context.CancelFunc, grant provider.Grant, result chan<- error) {
	ticker := time.NewTicker(h.renewEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			result <- nil
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(ctx, h.renewTimeout)
			err := h.store.RenewLease(renewCtx, grant)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					result <- nil
					return
				}
				cancelProvider()
				result <- err
				return
			}
		}
	}
}

func (h *Handler) authenticate(r *http.Request) (corev2.Caller, int, error) {
	if r == nil || h == nil || h.auth == nil {
		return corev2.Caller{}, http.StatusServiceUnavailable, errors.New("authentication unavailable")
	}
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return corev2.Caller{}, http.StatusUnauthorized, errors.New("invalid service key")
	}
	raw, ok := strings.CutPrefix(values[0], "Bearer ")
	if !ok || raw == "" || strings.TrimSpace(raw) != raw {
		return corev2.Caller{}, http.StatusUnauthorized, errors.New("invalid service key")
	}
	identity, err := h.auth.AuthenticateRaw(r.Context(), raw)
	if errors.Is(err, v2postgres.ErrInvalidServiceKey) {
		return corev2.Caller{}, http.StatusUnauthorized, err
	}
	if err != nil {
		return corev2.Caller{}, http.StatusServiceUnavailable, err
	}
	caller := corev2.Caller{
		AccountID: identity.AccountID, NamespaceID: identity.NamespaceID,
		ServiceKeyID: identity.ServiceKeyID, Operations: identity.Operations,
		AllowedAgentIDs: identity.AllowedAgentIDs, CanAssertScope: identity.CanAssertScope,
	}
	if !slices.Contains(caller.Operations, corev2.OperationInvoke) || len(caller.AllowedAgentIDs) == 0 || !caller.CanAssertScope {
		return corev2.Caller{}, http.StatusForbidden, corev2.ErrUnauthorized
	}
	return caller, 0, nil
}

func (h *Handler) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, corev2.ErrUnauthorized):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, corev2.ErrInvalidArgument), errors.Is(err, provider.ErrUnsupportedEndpoint):
		writeError(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, corev2.ErrLimitExceeded):
		writeError(w, http.StatusTooManyRequests, "limit_exceeded")
	case errors.Is(err, corev2.ErrIdempotencyConflict), errors.Is(err, provider.ErrAlreadyInvoked), errors.Is(err, provider.ErrInvocationInProgress):
		writeError(w, http.StatusConflict, "invocation_conflict")
	case errors.Is(err, provider.ErrReservationUnavailable), errors.Is(err, provider.ErrRouteUnavailable):
		writeError(w, http.StatusServiceUnavailable, "admission_unavailable")
	default:
		h.logger.Error("v2 provider admission failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "admission_unavailable")
	}
}

func (h *Handler) abort(ctx context.Context, grant provider.Grant, uncertain bool) {
	h.complete(ctx, provider.CompleteRequest{Grant: grant, Uncertain: uncertain, FinishedAt: h.now().UTC()})
}

func (h *Handler) complete(ctx context.Context, req provider.CompleteRequest) {
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settlementTimeout)
	defer cancel()
	if err := h.store.Complete(settleCtx, req); err != nil {
		h.logger.Error("v2 provider settlement failed", "invocation_id", req.Grant.InvocationID, "error", err)
	}
}

func requestScope(header http.Header) (corev2.Scope, corev2.Ref, error) {
	value := func(name string, required bool) (corev2.Ref, error) {
		values := header.Values(name)
		if len(values) == 0 && !required {
			return "", nil
		}
		if len(values) != 1 {
			return "", fmt.Errorf("%s must occur exactly once", name)
		}
		ref := corev2.Ref(values[0])
		if err := ref.Validate(name, required); err != nil {
			return "", err
		}
		return ref, nil
	}
	tenant, err := value(HeaderTenant, true)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	billTo, err := value(HeaderBillTo, true)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	actor, err := value(HeaderActor, false)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	session, err := value(HeaderSession, false)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	feature, err := value(HeaderFeature, false)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	key, err := value(HeaderInvocationKey, true)
	if err != nil {
		return corev2.Scope{}, "", err
	}
	scope := corev2.Scope{Tenant: tenant, Actor: actor, BillTo: billTo, Session: session, Feature: feature}
	if err := scope.ValidateAdmission(); err != nil {
		return corev2.Scope{}, "", err
	}
	return scope, key, nil
}

func endpointFromPath(path string) (string, bool) {
	marker := "/openai/"
	index := strings.LastIndex(path, marker)
	if index < 0 {
		return "", false
	}
	endpoint := path[index+len(marker):]
	switch endpoint {
	case provider.EndpointChatCompletions, provider.EndpointResponses, provider.EndpointEmbeddings:
		return endpoint, true
	default:
		return "", false
	}
}

func readJSONDocument(body io.Reader) ([]byte, map[string]any, error) {
	raw, err := io.ReadAll(body)
	if err != nil || len(raw) == 0 {
		return nil, nil, errors.New("invalid JSON body")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, nil, errors.New("invalid JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("multiple JSON values")
	}
	return raw, document, nil
}

func optionalString(document map[string]any, key string) (string, error) {
	value, ok := document[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n") || len(text) > 255 {
		return "", errors.New("invalid string")
	}
	return text, nil
}

func outputUpperBound(endpoint string, document map[string]any) (int64, bool, error) {
	if endpoint == provider.EndpointEmbeddings {
		return 0, true, nil
	}
	keys := []string{"max_output_tokens"}
	if endpoint == provider.EndpointChatCompletions {
		keys = []string{"max_completion_tokens", "max_tokens"}
	}
	for _, key := range keys {
		value, ok := document[key]
		if !ok || value == nil {
			continue
		}
		number, ok := value.(json.Number)
		if !ok {
			return 0, false, errors.New("token maximum must be an integer")
		}
		bound, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil || bound <= 0 {
			return 0, false, errors.New("token maximum must be positive")
		}
		if endpoint == provider.EndpointChatCompletions {
			choices, err := positiveIntegerField(document, "n", 1)
			if err != nil {
				return 0, false, err
			}
			if bound > math.MaxInt64/choices {
				return 0, false, errors.New("token maximum overflows across choices")
			}
			bound *= choices
		}
		return bound, true, nil
	}
	return 0, false, nil
}

func positiveIntegerField(document map[string]any, key string, fallback int64) (int64, error) {
	value, ok := document[key]
	if !ok || value == nil {
		return fallback, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("integer field must be a number")
	}
	parsed, err := strconv.ParseInt(string(number), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("integer field must be a positive integer")
	}
	return parsed, nil
}

func containsUnboundedProviderInput(value any) bool {
	unsafeKeys := map[string]struct{}{
		"image_url": {}, "input_image": {}, "input_audio": {}, "audio": {},
		"file_id": {}, "input_file": {}, "video": {},
	}
	var visit func(any) bool
	visit = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, unsafe := unsafeKeys[strings.ToLower(key)]; unsafe {
					return true
				}
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func ensureStreamingUsage(endpoint string, document map[string]any) {
	stream, _ := document["stream"].(bool)
	if !stream || endpoint != provider.EndpointChatCompletions {
		return
	}
	options, _ := document["stream_options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	}
	options["include_usage"] = true
	document["stream_options"] = options
}

func invocationHash(agent corev2.Ref, endpoint string, scope corev2.Scope, key corev2.Ref, body []byte) [32]byte {
	bodyDigest := sha256.Sum256(body)
	canonical, _ := json.Marshal(struct {
		Agent    corev2.Ref   `json:"agent"`
		Endpoint string       `json:"endpoint"`
		Scope    corev2.Scope `json:"scope"`
		Key      corev2.Ref   `json:"key"`
		Body     string       `json:"body_sha256"`
	}{agent, endpoint, scope, key, hex.EncodeToString(bodyDigest[:])})
	return sha256.Sum256(canonical)
}

func providerURL(baseURL, endpoint string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", provider.ErrRouteUnavailable
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", provider.ErrRouteUnavailable
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + endpoint
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func outboundHeaders(in http.Header, credential []byte, invocationKey corev2.Ref) http.Header {
	out := make(http.Header)
	out.Set("Content-Type", "application/json")
	out.Set("Accept", "application/json, text/event-stream")
	if trace := in.Get("Traceparent"); trace != "" && !strings.ContainsAny(trace, "\r\n") {
		out.Set("Traceparent", trace)
	}
	out.Set("Authorization", "Bearer "+strings.TrimPrefix(string(credential), "Bearer "))
	digest := sha256.Sum256([]byte(invocationKey))
	out.Set("Idempotency-Key", "kave-"+hex.EncodeToString(digest[:16]))
	return out
}

func copyResponseHeaders(dst, src http.Header) {
	for _, name := range []string{
		"Content-Type", "Cache-Control", "Retry-After", "OpenAI-Processing-Ms",
		"OpenAI-Version", "X-Request-Id", "X-Ratelimit-Limit-Requests",
		"X-Ratelimit-Remaining-Requests", "X-Ratelimit-Reset-Requests",
	} {
		for _, value := range src.Values(name) {
			dst.Add(name, value)
		}
	}
}

func providerRequestID(header http.Header) string {
	for _, name := range []string{"X-Request-Id", "OpenAI-Request-Id"} {
		if value := header.Get(name); value != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n") {
			return value
		}
	}
	return ""
}

func copyProviderBody(dst http.ResponseWriter, src io.Reader, capture io.Writer, stream bool) error {
	buffer := make([]byte, 32<<10)
	controller := http.NewResponseController(dst)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			_, _ = capture.Write(buffer[:n])
			if _, err := dst.Write(buffer[:n]); err != nil {
				return err
			}
			if stream {
				_ = controller.Flush()
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

type tailCapture struct {
	maximum int
	data    []byte
}

func newTailCapture(maximum int) *tailCapture { return &tailCapture{maximum: maximum} }
func (c *tailCapture) Write(p []byte) (int, error) {
	n := len(p)
	if n >= c.maximum {
		c.data = append(c.data[:0], p[n-c.maximum:]...)
		return n, nil
	}
	if overflow := len(c.data) + n - c.maximum; overflow > 0 {
		copy(c.data, c.data[overflow:])
		c.data = c.data[:len(c.data)-overflow]
	}
	c.data = append(c.data, p...)
	return n, nil
}
func (c *tailCapture) Bytes() []byte { return c.data }

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
}

func errorCode(status int) string {
	if status == http.StatusUnauthorized {
		return "invalid_service_key"
	}
	if status == http.StatusForbidden {
		return "forbidden"
	}
	return "authentication_unavailable"
}
