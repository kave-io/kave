//go:build integration

package openai

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	connruntime "github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
)

// TestIntegrationOpenAI hits api.openai.com when OPENAI_API_KEY is set in
// the environment. It exercises PrepareRequest → real HTTP → ParseResponse
// for chat completions (non-stream), responses (non-stream), and chat
// completions streaming.
//
// Run with: OPENAI_API_KEY=sk-... go test -tags=integration ./core/connectors/llm/openai/
//
// Override the model with OPENAI_TEST_MODEL=gpt-4o-mini (defaults to
// gpt-4o-mini to keep cost negligible).
func TestIntegrationOpenAI(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	model := strings.TrimSpace(os.Getenv("OPENAI_TEST_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}

	client := &http.Client{Timeout: 60 * time.Second}
	conn := NewConnector(nil)

	header := func() http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		return h
	}

	t.Run("chat_completions", func(t *testing.T) {
		call := &connruntime.LLMCall{
			Provider:     "openai",
			Method:       "POST",
			UpstreamPath: "/v1/chat/completions",
			Header:       header(),
			Body: []byte(`{
  "model": "` + model + `",
  "messages": [{"role":"user","content":"reply with one word: hello"}],
  "max_tokens": 8
}`),
		}
		result := liveCall(t, client, conn, call, key)
		if result.TokenUsage == nil || result.TokenUsage.InputTokens == 0 || result.TokenUsage.OutputTokens == 0 {
			t.Fatalf("usage not extracted from live response: %#v", result.TokenUsage)
		}
		if result.TokenUsage.Model == "" {
			t.Errorf("model not extracted from live response")
		}
		if result.Usage == nil || result.Usage.Tokens != result.TokenUsage {
			t.Errorf("Usage.Tokens not linked to TokenUsage")
		}
	})

	t.Run("responses", func(t *testing.T) {
		call := &connruntime.LLMCall{
			Provider:     "openai",
			Method:       "POST",
			UpstreamPath: "/v1/responses",
			Header:       header(),
			Body: []byte(`{
  "model": "` + model + `",
  "input": "reply with one word: hello",
  "max_output_tokens": 16
}`),
		}
		result := liveCall(t, client, conn, call, key)
		if result.TokenUsage == nil || result.TokenUsage.InputTokens == 0 || result.TokenUsage.OutputTokens == 0 {
			t.Fatalf("usage not extracted from live /v1/responses: %#v", result.TokenUsage)
		}
	})

	t.Run("chat_completions_stream", func(t *testing.T) {
		call := &connruntime.LLMCall{
			Provider:     "openai",
			Method:       "POST",
			UpstreamPath: "/v1/chat/completions",
			Header:       header(),
			Body: []byte(`{
  "model": "` + model + `",
  "stream": true,
  "stream_options": {"include_usage": true},
  "messages": [{"role":"user","content":"reply with one word: hello"}],
  "max_tokens": 8
}`),
		}
		result := liveCallContentType(t, client, conn, call, key, "text/event-stream")
		if result.TokenUsage == nil || result.TokenUsage.InputTokens == 0 || result.TokenUsage.OutputTokens == 0 {
			t.Fatalf("usage not aggregated from live SSE: %#v", result.TokenUsage)
		}
	})
}

// TestIntegrationCodexChatGPT hits chatgpt.com/backend-api/codex/responses
// using ~/.codex/auth.json (or $KAVE_CODEX_AUTH_PATH). It exercises the
// connector's self-acquire credential branch end-to-end. Skips automatically
// if no codex auth is present on disk.
//
// Run with: go test -tags=integration ./core/connectors/llm/openai/ -run TestIntegrationCodexChatGPT -v
func TestIntegrationCodexChatGPT(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("KAVE_CODEX_AUTH_PATH"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home dir: %v", err)
		}
		path = home + "/.codex/auth.json"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("codex auth not present at %s — log into codex CLI first", path)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	conn := NewConnector(nil)

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	call := &connruntime.LLMCall{
		Provider:        "openai",
		Method:          "POST",
		UpstreamBaseURL: "https://chatgpt.com",
		UpstreamPath:    "/backend-api/codex/responses",
		Header:          h,
		Body: []byte(`{
  "model": "gpt-5.5",
  "instructions": "You are a terse assistant. Reply with a single word.",
  "input": [{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "reply with one word: hello"}]}],
  "stream": true,
  "store": false
}`),
	}

	// Empty credential → connector loads ~/.codex/auth.json itself and
	// injects the chatgpt-account-id / OpenAI-Beta / originator / version
	// headers chatgpt.com requires. Codex requires streaming.
	result := liveCallContentType(t, client, conn, call, "", "text/event-stream")
	if result == nil || len(result.Body) == 0 {
		t.Fatal("empty response from chatgpt.com")
	}
	// Usage may or may not be populated for a Codex passthrough response —
	// the format mirrors /v1/responses, so token counts are expected.
	if result.TokenUsage == nil || (result.TokenUsage.InputTokens == 0 && result.TokenUsage.OutputTokens == 0) {
		t.Fatalf("usage not extracted from codex response; last 512 bytes: %s",
			truncate(result.Body[max(0, len(result.Body)-512):], 512))
	}
	t.Logf("codex usage: input=%d output=%d reasoning=%d model=%q",
		result.TokenUsage.InputTokens, result.TokenUsage.OutputTokens,
		result.TokenUsage.Reasoning, result.TokenUsage.Model)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func liveCall(t *testing.T, client *http.Client, conn *Connector, call *connruntime.LLMCall, key string) *pipeline.Result {
	return liveCallContentType(t, client, conn, call, key, "application/json")
}

func liveCallContentType(t *testing.T, client *http.Client, conn *Connector, call *connruntime.LLMCall, key, contentType string) *pipeline.Result {
	t.Helper()
	prep, err := conn.PrepareRequest(call, key)
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	req, err := http.NewRequest(prep.Method, prep.URL, bytes.NewReader(prep.Body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, vs := range prep.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if strings.Contains(contentType, "event-stream") {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upstream call: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("upstream %d: %s", resp.StatusCode, string(body))
	}
	respCT := resp.Header.Get("Content-Type")
	if respCT == "" {
		respCT = contentType
	}
	result, err := conn.ParseResponse(body, respCT)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	return result
}
