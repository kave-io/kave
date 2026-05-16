package openai

import (
	"net/http"
	"testing"

	"github.com/kave-io/kave/core/connectors/cert"
	connruntime "github.com/kave-io/kave/core/connectors/runtime"
)

// TestCertSuite runs the full connector certification suite against the
// OpenAI LLM connector. This is the gold-standard run — every other LLM
// connector must pass an equivalent suite before shipping.
//
// Fixtures use the post-strip upstream paths the gateway hands to the
// connector (e.g. "/v1/chat/completions"), NOT inbound Kave routes
// ("/v1/openai/chat/completions"). The gateway translates between them
// in server/internal/gateway/connectors/framework/raw.go.
func TestCertSuite(t *testing.T) {
	conn := NewConnector(nil)

	chatCompletionsBody := []byte(`{
  "model": "gpt-4o",
  "choices": [
    {"message": {"tool_calls": [
      {"id": "call_1", "type": "function", "function": {"name": "lookup_repo", "arguments": "{\"repo\":\"kave\"}"}}
    ]}}
  ],
  "usage": {
    "prompt_tokens": 120,
    "completion_tokens": 30,
    "prompt_tokens_details": {"cached_tokens": 80},
    "completion_tokens_details": {"reasoning_tokens": 5}
  }
}`)

	responsesBody := []byte(`{
  "model": "gpt-5.4",
  "output": [
    {"type": "function_call", "call_id": "call_2", "name": "create_issue", "arguments": "{\"title\":\"bug\"}"}
  ],
  "usage": {
    "input_tokens": 55,
    "output_tokens": 13,
    "input_tokens_details": {"cached_tokens": 21},
    "output_tokens_details": {"reasoning_tokens": 8}
  }
}`)

	chatSSEBody := []byte(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_3\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"openai\\\"}\"}}]}}]}\n\n" +
			"data: {\"model\":\"gpt-4o\",\"usage\":{\"prompt_tokens\":42,\"completion_tokens\":7}}\n\n" +
			"data: [DONE]\n\n",
	)

	// Responses API SSE stream: function call arguments arrive across deltas,
	// followed by a final response.completed event with the usage block.
	responsesSSEBody := []byte(
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_r1\",\"name\":\"create_issue\"}}\n\n" +
			"data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call_r1\",\"output_index\":0,\"name\":\"create_issue\",\"delta\":\"{\\\"title\\\":\\\"\"}\n\n" +
			"data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call_r1\",\"output_index\":0,\"name\":\"create_issue\",\"delta\":\"bug\\\"}\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.4\",\"usage\":{\"input_tokens\":91,\"output_tokens\":17,\"output_tokens_details\":{\"reasoning_tokens\":4}}}}\n\n" +
			"data: [DONE]\n\n",
	)

	// A typical OpenAI error envelope. ParseResponse should not panic, and
	// since there is no usage block it should return zero TokenUsage.
	errorBody := []byte(`{"error":{"message":"invalid_api_key","type":"invalid_request_error","code":"invalid_api_key"}}`)

	header := func() http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		return h
	}

	cert.RunLLM(t, cert.LLMSpec{
		Name:       "openai",
		Connector:  conn,
		Descriptor: conn,
		GoldenDir:  "testdata/cert",
		Cases: []cert.LLMCase{
			{
				Name: "chat_completions_basic",
				Call: &connruntime.LLMCall{
					Provider:     "openai",
					Method:       "POST",
					UpstreamPath: "/v1/chat/completions",
					Header:       header(),
					Body:         []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`),
				},
				Credential:        "sk-test-key",
				ExpectURLContains: "https://api.openai.com/v1/chat/completions",
				ResponseBody:      chatCompletionsBody,
				ResponseType:      "application/json",
				ExpectModel:       "gpt-4o",
				ExpectInputTok:    120,
				ExpectOutputTok:   30,
				ExpectCacheRead:   80,
				ExpectReasoning:   5,
				ExpectToolCalls:   []cert.ToolCallExpect{{Name: "lookup_repo", ArgsContain: `"repo":"kave"`}},
			},
			{
				Name: "responses_function_call",
				Call: &connruntime.LLMCall{
					Provider:     "openai",
					Method:       "POST",
					UpstreamPath: "/v1/responses",
					Header:       header(),
					Body:         []byte(`{"model":"gpt-5.4","input":"hi"}`),
				},
				Credential:        "sk-test-key",
				ExpectURLContains: "https://api.openai.com/v1/responses",
				ResponseBody:      responsesBody,
				ResponseType:      "application/json",
				ExpectModel:       "gpt-5.4",
				ExpectInputTok:    55,
				ExpectOutputTok:   13,
				ExpectCacheRead:   21,
				ExpectReasoning:   8,
				ExpectToolCalls:   []cert.ToolCallExpect{{Name: "create_issue", ArgsContain: `"title":"bug"`}},
			},
			{
				Name: "chat_completions_sse_stream",
				Call: &connruntime.LLMCall{
					Provider:     "openai",
					Method:       "POST",
					UpstreamPath: "/v1/chat/completions",
					Header:       header(),
					Body:         []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`),
				},
				Credential:        "sk-test-key",
				ExpectURLContains: "https://api.openai.com/v1/chat/completions",
				ResponseBody:      chatSSEBody,
				ResponseType:      "text/event-stream",
				Streaming:         true,
				ExpectInputTok:    42,
				ExpectOutputTok:   7,
				ExpectToolCalls:   []cert.ToolCallExpect{{Name: "search", ArgsContain: `"q":"openai"`}},
			},
			{
				Name: "responses_sse_stream",
				Call: &connruntime.LLMCall{
					Provider:     "openai",
					Method:       "POST",
					UpstreamPath: "/v1/responses",
					Header:       header(),
					Body:         []byte(`{"model":"gpt-5.4","stream":true,"input":"hi"}`),
				},
				Credential:        "sk-test-key",
				ExpectURLContains: "https://api.openai.com/v1/responses",
				ResponseBody:      responsesSSEBody,
				ResponseType:      "text/event-stream",
				Streaming:         true,
				ExpectInputTok:    91,
				ExpectOutputTok:   17,
				ExpectReasoning:   4,
				ExpectToolCalls:   []cert.ToolCallExpect{{Name: "create_issue", ArgsContain: `"title":"bug"`}},
			},
			{
				Name: "codex_chatgpt_passthrough",
				Call: &connruntime.LLMCall{
					Provider:        "openai",
					Method:          "POST",
					UpstreamBaseURL: "https://chatgpt.com",
					UpstreamPath:    "/backend-api/codex/responses",
					Header:          header(),
					Body:            []byte(`{"model":"gpt-5-codex","input":"hi"}`),
				},
				// Supplying an explicit credential bypasses codexauth.Load() so this
				// case runs in CI without ~/.codex/auth.json on disk.
				Credential:        "chatgpt-token",
				ExpectURLContains: "https://chatgpt.com/backend-api/codex/responses",
				ResponseBody:      responsesBody,
				ResponseType:      "application/json",
				ExpectInputTok:    55,
				ExpectOutputTok:   13,
				ExpectToolCalls:   []cert.ToolCallExpect{{Name: "create_issue"}},
			},
			// codex_sse_stream exercises the response.completed usage fallback path
			// (response.usage.*) that was silently dropping tokens before the fix in
			// parseJSONUsage. Keeping it in the cert suite ensures any future change
			// to the Codex SSE path must explicitly break this test to regress.
			{
				Name: "codex_sse_stream",
				Call: &connruntime.LLMCall{
					Provider:        "openai",
					Method:          "POST",
					UpstreamBaseURL: "https://chatgpt.com",
					UpstreamPath:    "/backend-api/codex/responses",
					Header:          header(),
					Body:            []byte(`{"model":"gpt-5-codex","stream":true,"input":"hi"}`),
				},
				Credential:        "chatgpt-token",
				ExpectURLContains: "chatgpt.com/backend-api/codex/responses",
				ResponseBody: []byte(
					"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_c1\",\"name\":\"run_shell\"}}\n\n" +
						"data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call_c1\",\"name\":\"run_shell\",\"delta\":\"{\\\"cmd\\\":\\\"\"}\n\n" +
						"data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call_c1\",\"name\":\"run_shell\",\"delta\":\"ls\\\"}\"}\n\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5-codex\",\"usage\":{\"input_tokens\":33,\"output_tokens\":9}}}\n\n" +
						"data: [DONE]\n\n",
				),
				ResponseType:    "text/event-stream",
				Streaming:       true,
				ExpectInputTok:  33,
				ExpectOutputTok: 9,
				ExpectToolCalls: []cert.ToolCallExpect{{Name: "run_shell", ArgsContain: `"cmd":"ls"`}},
			},
			{
				Name: "upstream_error_envelope",
				Call: &connruntime.LLMCall{
					Provider:     "openai",
					Method:       "POST",
					UpstreamPath: "/v1/chat/completions",
					Header:       header(),
					Body:         []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`),
				},
				Credential:        "sk-test-key",
				ExpectURLContains: "https://api.openai.com/v1/chat/completions",
				ResponseBody:      errorBody,
				ResponseType:      "application/json",
				ExpectNoToolCalls: true,
				// No ExpectInputTok/OutputTok: parser must not invent usage for an error body.
			},
		},
	})
}
