package openai

import (
	"net/http"
	"testing"

	"github.com/kave-io/kave/core/connectors/runtime"
)

func TestParseChatResponseUsageCacheAndToolCalls(t *testing.T) {
	conn := NewConnector(nil)
	body := []byte(`{
		"model":"gpt-4o",
		"choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup_repo","arguments":"{\"repo\":\"kave\"}"}}]}}],
		"usage":{
			"prompt_tokens":100,
			"completion_tokens":20,
			"prompt_tokens_details":{"cached_tokens":80},
			"completion_tokens_details":{"reasoning_tokens":5}
		}
	}`)

	result, err := conn.ParseResponse(body, "application/json")
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if result.TokenUsage == nil {
		t.Fatal("expected token usage")
	}
	if result.TokenUsage.InputTokens != 100 || result.TokenUsage.OutputTokens != 20 {
		t.Fatalf("tokens = %d/%d", result.TokenUsage.InputTokens, result.TokenUsage.OutputTokens)
	}
	if result.TokenUsage.CacheRead != 80 || result.TokenUsage.Reasoning != 5 {
		t.Fatalf("cache/reasoning = %d/%d", result.TokenUsage.CacheRead, result.TokenUsage.Reasoning)
	}
	if len(result.ObservedSpans) != 1 {
		t.Fatalf("observed spans = %d, want 1", len(result.ObservedSpans))
	}
	if result.ObservedSpans[0].Connector != "tool_call" || result.ObservedSpans[0].Method != "function" {
		t.Fatalf("unexpected observed span: %#v", result.ObservedSpans[0])
	}
}

func TestParseResponsesUsageAndFunctionCall(t *testing.T) {
	conn := NewConnector(nil)
	body := []byte(`{
		"model":"gpt-5.4",
		"output":[{"type":"function_call","call_id":"call_2","name":"create_issue","arguments":"{\"title\":\"bug\"}"}],
		"usage":{
			"input_tokens":55,
			"output_tokens":13,
			"input_tokens_details":{"cached_tokens":21},
			"output_tokens_details":{"reasoning_tokens":8}
		}
	}`)

	result, err := conn.ParseResponse(body, "application/json")
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if result.TokenUsage.Model != "gpt-5.4" {
		t.Fatalf("model = %q", result.TokenUsage.Model)
	}
	if result.TokenUsage.InputTokens != 55 || result.TokenUsage.OutputTokens != 13 || result.TokenUsage.CacheRead != 21 {
		t.Fatalf("usage = %#v", result.TokenUsage)
	}
	if len(result.ObservedSpans) != 1 || result.ObservedSpans[0].Name != "tool.create_issue" {
		t.Fatalf("observed spans = %#v", result.ObservedSpans)
	}
}

func TestParseStreamingUsageAndToolCallDelta(t *testing.T) {
	conn := NewConnector(nil)
	body := []byte("data: {\"model\":\"gpt-4o\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_3\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"kave\\\"}\"}}]}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n" +
		"data: [DONE]\n\n")

	result, err := conn.ParseResponse(body, "text/event-stream")
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if result.TokenUsage == nil || result.TokenUsage.InputTokens != 7 || result.TokenUsage.CacheRead != 2 {
		t.Fatalf("usage = %#v", result.TokenUsage)
	}
	if len(result.ObservedSpans) != 1 {
		t.Fatalf("observed spans = %d", len(result.ObservedSpans))
	}
	if got := string(*result.ObservedSpans[0].Input); got != `{"q":"kave"}` {
		t.Fatalf("tool args = %q", got)
	}
}

func TestPrepareRequestStripsCallerAuth(t *testing.T) {
	conn := NewConnector(&Config{BaseURL: "https://example.test/base"})
	call := &runtime.LLMCall{
		Method:       http.MethodPost,
		UpstreamPath: "/v1/responses",
		RawQuery:     "debug=1",
		Header:       http.Header{"Authorization": {"Bearer caller"}, "Connection": {"close"}, "Content-Type": {"application/json"}},
		Body:         []byte(`{}`),
	}

	req, err := conn.PrepareRequest(call, "provider-key")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if req.URL != "https://example.test/base/v1/responses?debug=1" {
		t.Fatalf("url = %q", req.URL)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer provider-key" {
		t.Fatalf("authorization = %q", got)
	}
	if got := req.Header.Get("Connection"); got != "" {
		t.Fatalf("connection header leaked: %q", got)
	}
}
