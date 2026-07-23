package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxValidationResponseBytes = 1 << 20

var (
	ErrAdapterUnavailable = errors.New("kave v2: provider adapter unavailable")
	ErrValidationRejected = errors.New("kave v2: provider validation rejected")
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ValidationTarget struct {
	Provider   string
	Protocol   string
	BaseURL    string
	Model      string
	Credential []byte
}

type ValidationEvidence struct {
	HTTPStatus        int
	ProviderRequestID string
}

// Adapter owns every provider-shaped concern at the data-plane boundary:
// authentication, a payload-free live credential/model probe, and response
// usage parsing. Adapters never persist or log request/response bodies.
type Adapter interface {
	Protocol() string
	ApplyAuthentication(http.Header, []byte) error
	Validate(context.Context, HTTPDoer, ValidationTarget) (ValidationEvidence, error)
	ParseUsage([]byte, string, string) (Usage, error)
}

type Registry struct {
	byProvider map[string]Adapter
	byProtocol map[string]Adapter
}

func NewRegistry(providerAdapters map[string]Adapter, protocolAdapters map[string]Adapter) (*Registry, error) {
	registry := &Registry{byProvider: map[string]Adapter{}, byProtocol: map[string]Adapter{}}
	for name, adapter := range providerAdapters {
		if adapter == nil || cleanAdapterKey(name) == "" {
			return nil, errors.New("kave v2: provider adapter names and implementations are required")
		}
		registry.byProvider[cleanAdapterKey(name)] = adapter
	}
	for protocol, adapter := range protocolAdapters {
		if adapter == nil || cleanAdapterKey(protocol) == "" || cleanAdapterKey(adapter.Protocol()) != cleanAdapterKey(protocol) {
			return nil, errors.New("kave v2: protocol adapter registration is invalid")
		}
		registry.byProtocol[cleanAdapterKey(protocol)] = adapter
	}
	return registry, nil
}

func DefaultRegistry() *Registry {
	openAI := OpenAIAdapter{}
	registry, _ := NewRegistry(map[string]Adapter{"openai": openAI}, map[string]Adapter{"openai": openAI})
	return registry
}

func (r *Registry) Resolve(providerName, protocol string) (Adapter, error) {
	if r == nil {
		return nil, ErrAdapterUnavailable
	}
	if adapter := r.byProvider[cleanAdapterKey(providerName)]; adapter != nil {
		return adapter, nil
	}
	if protocol == "" {
		protocol = "openai"
	}
	if adapter := r.byProtocol[cleanAdapterKey(protocol)]; adapter != nil {
		return adapter, nil
	}
	return nil, ErrAdapterUnavailable
}

func cleanAdapterKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// OpenAIAdapter also serves explicitly configured OpenAI-compatible routes;
// they opt into the same protocol, bearer authentication, model endpoint, and
// usage envelope rather than inheriting behavior accidentally in the handler.
type OpenAIAdapter struct{}

func (OpenAIAdapter) Protocol() string { return "openai" }

func (OpenAIAdapter) ApplyAuthentication(header http.Header, credential []byte) error {
	if header == nil || len(credential) == 0 {
		return ErrAdapterUnavailable
	}
	for _, value := range credential {
		if value < 0x21 || value > 0x7e {
			return ErrAdapterUnavailable
		}
	}
	header.Set("Authorization", "Bearer "+strings.TrimPrefix(string(credential), "Bearer "))
	return nil
}

func (adapter OpenAIAdapter) Validate(ctx context.Context, client HTTPDoer, target ValidationTarget) (ValidationEvidence, error) {
	if client == nil || target.Model == "" || len(target.Credential) == 0 {
		return ValidationEvidence{}, ErrValidationRejected
	}
	endpoint, err := openAIModelsURL(target.BaseURL)
	if err != nil {
		return ValidationEvidence{}, ErrValidationRejected
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ValidationEvidence{}, ErrValidationRejected
	}
	request.Header.Set("Accept", "application/json")
	if err := adapter.ApplyAuthentication(request.Header, target.Credential); err != nil {
		return ValidationEvidence{}, ErrValidationRejected
	}
	response, err := client.Do(request)
	if err != nil {
		return ValidationEvidence{}, ErrValidationRejected
	}
	defer response.Body.Close()
	evidence := ValidationEvidence{
		HTTPStatus: response.StatusCode, ProviderRequestID: safeProviderRequestID(response.Header),
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxValidationResponseBytes+1))
	if err != nil || len(body) > maxValidationResponseBytes {
		clear(body)
		return evidence, ErrValidationRejected
	}
	defer clear(body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return evidence, ErrValidationRejected
	}
	// Validate the model exists via the list endpoint rather than the
	// retrieve-single-model endpoint: many OpenAI-compatible providers (e.g.
	// GapGPT) do not implement GET /models/{id} and answer 404 or a 200 error
	// body. This stays payload-free and rides the same pinned egress boundary.
	var document struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return evidence, ErrValidationRejected
	}
	for _, m := range document.Data {
		if m.ID == target.Model {
			return evidence, nil
		}
	}
	return evidence, ErrValidationRejected
}

func (OpenAIAdapter) ParseUsage(body []byte, contentType, fallbackModel string) (Usage, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Usage{}, ErrValidationRejected
	}
	var usage Usage
	found := false
	switch {
	case mediaType == "text/event-stream":
		usage, found, err = parseOpenAIEventStreamUsage(body)
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		usage, found, err = parseOpenAIJSONUsage(body)
	default:
		return Usage{}, ErrValidationRejected
	}
	if err != nil || !found {
		return Usage{}, ErrValidationRejected
	}
	if usage.Model == "" {
		usage.Model = fallbackModel
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 ||
		usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 ||
		usage.CacheReadTokens > usage.InputTokens || usage.CacheWriteTokens > usage.InputTokens ||
		usage.ReasoningTokens > usage.OutputTokens {
		return Usage{}, ErrValidationRejected
	}
	usage.Reported = true
	return usage, nil
}

func parseOpenAIEventStreamUsage(body []byte) (Usage, bool, error) {
	var result Usage
	found := false
	for _, rawLine := range bytes.Split(body, []byte{'\n'}) {
		line := bytes.TrimSpace(rawLine)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		usage, hasUsage, err := parseOpenAIJSONUsage(payload)
		if err != nil {
			// A tail capture can begin in the middle of an older event. Continue
			// until the complete terminal usage event instead of trusting it.
			continue
		}
		if hasUsage {
			result = usage
			found = true
		}
	}
	return result, found, nil
}

func parseOpenAIJSONUsage(body []byte) (Usage, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || document == nil {
		return Usage{}, false, ErrValidationRejected
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Usage{}, false, ErrValidationRejected
	}

	usageObject, ok := objectAt(document, "usage")
	model := stringAt(document, "model")
	if !ok {
		response, responseOK := objectAt(document, "response")
		if !responseOK {
			return Usage{Model: model}, false, nil
		}
		usageObject, ok = objectAt(response, "usage")
		if model == "" {
			model = stringAt(response, "model")
		}
	}
	if !ok {
		return Usage{Model: model}, false, nil
	}

	input, err := firstUsageInteger(usageObject, "prompt_tokens", "input_tokens")
	if err != nil {
		return Usage{}, false, err
	}
	output, err := firstUsageInteger(usageObject, "completion_tokens", "output_tokens")
	if err != nil {
		return Usage{}, false, err
	}
	inputDetails := firstObject(usageObject, "prompt_tokens_details", "input_tokens_details")
	outputDetails := firstObject(usageObject, "completion_tokens_details", "output_tokens_details")
	cacheRead, err := firstUsageInteger(inputDetails, "cached_tokens", "cache_read_tokens")
	if err != nil {
		return Usage{}, false, err
	}
	cacheWrite, err := firstUsageInteger(inputDetails,
		"cache_write_tokens", "cache_creation_tokens", "cached_creation_tokens")
	if err != nil {
		return Usage{}, false, err
	}
	reasoning, err := firstUsageInteger(outputDetails, "reasoning_tokens")
	if err != nil {
		return Usage{}, false, err
	}
	return Usage{
		InputTokens: input, OutputTokens: output,
		CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite,
		ReasoningTokens: reasoning, Model: model,
	}, true, nil
}

func objectAt(document map[string]any, key string) (map[string]any, bool) {
	if document == nil {
		return nil, false
	}
	value, ok := document[key].(map[string]any)
	return value, ok
}

func firstObject(document map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := objectAt(document, key); ok {
			return value
		}
	}
	return nil
}

func stringAt(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func firstUsageInteger(document map[string]any, keys ...string) (int64, error) {
	for _, key := range keys {
		value, exists := document[key]
		if !exists || value == nil {
			continue
		}
		number, ok := value.(json.Number)
		if !ok {
			return 0, fmt.Errorf("%w: usage field %s is not an integer", ErrValidationRejected, key)
		}
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("%w: usage field %s is invalid", ErrValidationRejected, key)
		}
		return parsed, nil
	}
	return 0, nil
}

func openAIModelsURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", ErrValidationRejected
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && providerLoopbackHost(parsed.Hostname())) {
		return "", ErrValidationRejected
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	baseEscapedPath := strings.TrimRight(parsed.EscapedPath(), "/")
	// Payload-free model-list endpoint. No user-controlled query, fragment, or
	// authority is introduced.
	parsed.Path = basePath + "/models"
	parsed.RawPath = baseEscapedPath + "/models"
	if parsed.RawPath == parsed.Path {
		parsed.RawPath = ""
	}
	return parsed.String(), nil
}

func providerLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func safeProviderRequestID(header http.Header) string {
	for _, name := range []string{"X-Request-Id", "OpenAI-Request-Id"} {
		value := header.Get(name)
		if value != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n") {
			return value
		}
	}
	return ""
}

var _ Adapter = OpenAIAdapter{}
