package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func TestDefaultRegistryResolvesProviderAndExplicitProtocol(t *testing.T) {
	t.Parallel()
	registry := DefaultRegistry()
	for _, test := range []struct{ provider, protocol string }{
		{provider: "openai", protocol: "openai"},
		{provider: "private-openai-compatible", protocol: "openai"},
		{provider: "private-openai-compatible", protocol: ""},
	} {
		adapter, err := registry.Resolve(test.provider, test.protocol)
		if err != nil || adapter.Protocol() != "openai" {
			t.Fatalf("Resolve(%q, %q) = %T, %v", test.provider, test.protocol, adapter, err)
		}
	}
	if _, err := registry.Resolve("unknown", "anthropic"); !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("unknown adapter error = %v", err)
	}
}

func TestOpenAIAdapterValidatesCredentialAndExactModelWithoutPayload(t *testing.T) {
	t.Parallel()
	var sawBody bool
	client := httpDoerFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Errorf("validation request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.Body != nil && r.ContentLength != 0 {
			sawBody = true
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Request-Id": []string{"probe-123"}, "Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"other-model"},{"id":"gpt-safe","object":"model"}]}`)),
		}, nil
	})

	evidence, err := (OpenAIAdapter{}).Validate(context.Background(), client, ValidationTarget{
		Provider: "openai", Protocol: "openai", BaseURL: "https://api.example.com/v1",
		Model: "gpt-safe", Credential: []byte("provider-secret"),
	})
	if err != nil || evidence.HTTPStatus != 200 || evidence.ProviderRequestID != "probe-123" || sawBody {
		t.Fatalf("Validate() = %+v, %v; sawBody=%v", evidence, err, sawBody)
	}
}

func TestOpenAIAdapterValidationRejectsProviderErrorsAndWrongModel(t *testing.T) {
	t.Parallel()
	for _, response := range []struct {
		status int
		body   string
	}{
		{status: http.StatusUnauthorized, body: `{"error":{"message":"contains-sensitive-provider-detail"}}`},
		{status: http.StatusOK, body: `{"object":"list","data":[{"id":"different-model"}]}`},
		{status: http.StatusOK, body: `not-json`},
		{status: http.StatusOK, body: strings.Repeat("x", maxValidationResponseBytes+1)},
	} {
		client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: response.status, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(response.body)),
			}, nil
		})
		_, err := (OpenAIAdapter{}).Validate(context.Background(), client, ValidationTarget{
			BaseURL: "https://api.example.com/v1", Model: "gpt-safe", Credential: []byte("secret"),
		})
		if !errors.Is(err, ErrValidationRejected) || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("validation error = %v", err)
		}
	}
	_, err := (OpenAIAdapter{}).Validate(context.Background(), httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("sensitive transport detail")
	}), ValidationTarget{
		BaseURL: "https://api.example.com/v1", Model: "gpt-safe", Credential: []byte("secret"),
	})
	if !errors.Is(err, ErrValidationRejected) || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("transport validation error = %v", err)
	}
}

func TestOpenAIAdapterParsesDetailedUsage(t *testing.T) {
	t.Parallel()
	usage, err := (OpenAIAdapter{}).ParseUsage([]byte(`{
  "model":"gpt-5",
  "usage":{
    "prompt_tokens":100,
    "completion_tokens":20,
    "prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":10},
    "completion_tokens_details":{"reasoning_tokens":5}
  }
}`), "application/json", "fallback")
	if err != nil || !usage.Reported || usage.Model != "gpt-5" || usage.InputTokens != 100 ||
		usage.OutputTokens != 20 || usage.CacheReadTokens != 40 || usage.CacheWriteTokens != 10 || usage.ReasoningTokens != 5 {
		t.Fatalf("ParseUsage() = %+v, %v", usage, err)
	}
}

func TestOpenAIAdapterParsesTerminalSSEUsage(t *testing.T) {
	t.Parallel()
	body := []byte("event: response.output_text.delta\r\n" +
		"data: {not-a-complete-tail-event}\r\n\r\n" +
		"event: response.completed\r\n" +
		`data: {"type":"response.completed","response":{"model":"gpt-stream","usage":{"input_tokens":21,"output_tokens":8,"input_tokens_details":{"cached_tokens":5,"cache_creation_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}}` + "\r\n\r\n" +
		"data: [DONE]\r\n")

	usage, err := (OpenAIAdapter{}).ParseUsage(body, "text/event-stream; charset=utf-8", "fallback")
	if err != nil || !usage.Reported || usage.Model != "gpt-stream" || usage.InputTokens != 21 ||
		usage.OutputTokens != 8 || usage.CacheReadTokens != 5 || usage.CacheWriteTokens != 3 ||
		usage.ReasoningTokens != 2 {
		t.Fatalf("ParseUsage(SSE) = %+v, %v", usage, err)
	}
}

func TestOpenAIAdapterUsageMediaTypes(t *testing.T) {
	t.Parallel()
	adapter := OpenAIAdapter{}
	for _, contentType := range []string{
		"application/json; charset=utf-8",
		"application/vnd.openai.response+json",
	} {
		usage, err := adapter.ParseUsage([]byte(`{"usage":{"input_tokens":1,"output_tokens":0}}`), contentType, "fallback")
		if err != nil || usage.Model != "fallback" || !usage.Reported {
			t.Fatalf("ParseUsage(%q) = %+v, %v", contentType, usage, err)
		}
	}

	for _, contentType := range []string{"", "text/plain", "application/json; charset==utf-8"} {
		if _, err := adapter.ParseUsage([]byte(`{"usage":{"input_tokens":1}}`), contentType, "fallback"); !errors.Is(err, ErrValidationRejected) {
			t.Fatalf("ParseUsage(%q) error = %v, want validation rejected", contentType, err)
		}
	}
}

func TestOpenAIAdapterRejectsInvalidUsageIntegers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "fractional", body: `{"usage":{"input_tokens":1.5,"output_tokens":0}}`},
		{name: "exponent", body: `{"usage":{"input_tokens":1e3,"output_tokens":0}}`},
		{name: "overflow", body: `{"usage":{"input_tokens":9223372036854775808,"output_tokens":0}}`},
		{name: "negative", body: `{"usage":{"input_tokens":-1,"output_tokens":0}}`},
		{name: "string", body: `{"usage":{"input_tokens":"1","output_tokens":0}}`},
		{name: "cached exceeds input", body: `{"usage":{"input_tokens":1,"output_tokens":0,"input_tokens_details":{"cached_tokens":2}}}`},
		{name: "cache write exceeds input", body: `{"usage":{"input_tokens":1,"output_tokens":0,"input_tokens_details":{"cache_write_tokens":2}}}`},
		{name: "reasoning exceeds output", body: `{"usage":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{"reasoning_tokens":2}}}`},
		{name: "trailing document", body: `{"usage":{"input_tokens":1,"output_tokens":0}} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := (OpenAIAdapter{}).ParseUsage([]byte(test.body), "application/json", "fallback"); !errors.Is(err, ErrValidationRejected) {
				t.Fatalf("ParseUsage() error = %v, want validation rejected", err)
			}
		})
	}
}

func TestOpenAIAdapterRejectsCredentialHeaderInjection(t *testing.T) {
	t.Parallel()
	header := make(http.Header)
	if err := (OpenAIAdapter{}).ApplyAuthentication(header, []byte("secret\r\nX-Evil: yes")); err == nil || header.Get("Authorization") != "" {
		t.Fatalf("ApplyAuthentication() error=%v header=%q", err, header.Get("Authorization"))
	}
}

func TestOpenAIModelsURLTargetsTheListEndpoint(t *testing.T) {
	t.Parallel()
	got, err := openAIModelsURL("https://api.example.com/v1")
	if err != nil || got != "https://api.example.com/v1/models" {
		t.Fatalf("openAIModelsURL() = %q, %v", got, err)
	}
}
