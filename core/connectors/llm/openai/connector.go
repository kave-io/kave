package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/llm/openai/codexauth"
	"github.com/kave-io/kave/core/connectors/llm/shared"
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/tidwall/gjson"
)

const codexChatGPTPrefix = "/backend-api/codex/"

const (
	APIVersion  = "v1"
	defaultBase = "https://api.openai.com"
)

type Config struct {
	BaseURL string
}

type Connector struct {
	baseURL string
}

func NewConnector(cfg *Config) *Connector {
	base := defaultBase
	if cfg != nil && strings.TrimSpace(cfg.BaseURL) != "" {
		base = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	}
	return &Connector{baseURL: base}
}

func (c *Connector) Name() string { return "openai" }

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "openai" {
		return nil, fmt.Errorf("openai: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		Kind:             connectors.KindLLM,
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
		SupportedMethods: []string{
			"responses",
			"responses.streaming",
			"chat.completions",
			"chat.completions.streaming",
		},
		SupportedRoutes: []string{
			"/v1/openai/responses",
			"/v1/openai/chat/completions",
		},
		RequiresAuth:  true,
		CanProxy:      true,
		StreamSupport: true,
		APIVersion:    APIVersion,
	}
}

func (c *Connector) PrepareRequest(call *runtime.LLMCall, credential string) (*runtime.PreparedRequest, error) {
	baseURL := c.baseURL
	if strings.TrimSpace(call.UpstreamBaseURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(call.UpstreamBaseURL), "/")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = singleSlashJoin(base.Path, call.UpstreamPath)
	base.RawQuery = call.RawQuery

	headers := runtime.CloneHeader(call.Header)
	preserveUpgrade := strings.EqualFold(headers.Get("Upgrade"), "websocket")
	stripHopHeaders(headers, preserveUpgrade)

	if credential == "" && strings.HasPrefix(call.UpstreamPath, codexChatGPTPrefix) {
		auth, authErr := codexauth.Load()
		if authErr != nil {
			return nil, fmt.Errorf("%w: codex chatgpt auth: %v", runtime.ErrCredentialRequired, authErr)
		}
		credential = auth.AccessToken
		if auth.AccountID != "" && headers.Get("chatgpt-account-id") == "" {
			headers.Set("chatgpt-account-id", auth.AccountID)
		}
		if headers.Get("OpenAI-Beta") == "" {
			headers.Set("OpenAI-Beta", "responses=experimental")
		}
		if headers.Get("originator") == "" {
			headers.Set("originator", "codex_cli_rs")
		}
		if headers.Get("version") == "" {
			headers.Set("version", "0.0.0")
		}
	}

	if credential != "" {
		headers.Set("Authorization", "Bearer "+strings.TrimPrefix(credential, "Bearer "))
	}

	return &runtime.PreparedRequest{
		Method: call.Method,
		URL:    base.String(),
		Header: headers,
		Body:   call.Body,
	}, nil
}

func (c *Connector) ParseResponse(body []byte, contentType string) (*pipeline.Result, error) {
	result := &pipeline.Result{Body: body}
	var usage *coreruntime.TokenUsage
	var calls []runtime.ToolCall

	if strings.Contains(contentType, "event-stream") {
		usage, calls = parseSSE(body)
	} else {
		usage = parseJSONUsage(body)
		calls = parseJSONToolCalls(body)
	}
	if usage != nil {
		result.TokenUsage = usage
		result.Usage = &coreruntime.Usage{Tokens: usage, RequestCount: 1}
	}
	result.ObservedSpans = observedToolSpans(calls)
	return result, nil
}

func (c *Connector) RequiresAuth() bool { return true }

func singleSlashJoin(basePath, path string) string {
	basePath = strings.TrimRight(basePath, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if basePath == "" {
		return path
	}
	return basePath + path
}

func stripHopHeaders(headers http.Header, preserveUpgrade bool) {
	for _, key := range []string{
		"Authorization",
		"Connection",
		"Transfer-Encoding",
		"Accept-Encoding",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailer",
		"Upgrade",
	} {
		if preserveUpgrade && (key == "Connection" || key == "Upgrade") {
			continue
		}
		headers.Del(key)
	}
}

func parseJSONUsage(body []byte) *coreruntime.TokenUsage {
	model := firstString(
		gjson.GetBytes(body, "model"),
		gjson.GetBytes(body, "response.model"),
	)
	input := firstInt(
		gjson.GetBytes(body, "usage.prompt_tokens"),
		gjson.GetBytes(body, "usage.input_tokens"),
	)
	output := firstInt(
		gjson.GetBytes(body, "usage.completion_tokens"),
		gjson.GetBytes(body, "usage.output_tokens"),
	)
	cacheRead := firstInt(
		gjson.GetBytes(body, "usage.prompt_tokens_details.cached_tokens"),
		gjson.GetBytes(body, "usage.input_tokens_details.cached_tokens"),
	)
	cacheWrite := firstInt(
		gjson.GetBytes(body, "usage.prompt_tokens_details.cache_creation_tokens"),
		gjson.GetBytes(body, "usage.input_tokens_details.cache_write_tokens"),
		gjson.GetBytes(body, "usage.input_tokens_details.cached_creation_tokens"),
	)
	reasoning := firstInt(
		gjson.GetBytes(body, "usage.completion_tokens_details.reasoning_tokens"),
		gjson.GetBytes(body, "usage.output_tokens_details.reasoning_tokens"),
	)
	audioInput := firstInt(gjson.GetBytes(body, "usage.prompt_tokens_details.audio_tokens"))
	audioOutput := firstInt(gjson.GetBytes(body, "usage.completion_tokens_details.audio_tokens"))

	if input == 0 && output == 0 && cacheRead == 0 && cacheWrite == 0 && reasoning == 0 && model == "" {
		return nil
	}
	return &coreruntime.TokenUsage{
		InputTokens:  input,
		OutputTokens: output,
		CacheRead:    cacheRead,
		CacheWrite:   cacheWrite,
		Reasoning:    reasoning,
		AudioInput:   audioInput,
		AudioOutput:  audioOutput,
		Model:        model,
	}
}

func parseSSE(body []byte) (*coreruntime.TokenUsage, []runtime.ToolCall) {
	var usage *coreruntime.TokenUsage
	var calls []runtime.ToolCall
	acc := make(map[string]runtime.ToolCall)
	indexKeys := make(map[int]string)

	for _, line := range shared.SplitSSEDataLines(body) {
		if len(line) == 0 || line == "[DONE]" {
			continue
		}
		if u := parseJSONUsage([]byte(line)); u != nil {
			if usage == nil {
				usage = &coreruntime.TokenUsage{}
			}
			mergeUsage(usage, u)
		}
		if usage != nil && usage.Model == "" {
			usage.Model = gjson.Get(line, "model").String()
		}
		for _, call := range parseJSONToolCalls([]byte(line)) {
			key := callKey(call)
			if call.ID != "" {
				indexKeys[call.Index] = call.ID
			}
			if key == "" {
				key = indexKeys[call.Index]
			}
			if key == "" {
				key = strconv.Itoa(call.Index)
			}
			prev := acc[key]
			if prev.ID == "" {
				prev.ID = call.ID
			}
			if prev.Name == "" {
				prev.Name = call.Name
			}
			if prev.Type == "" {
				prev.Type = call.Type
			}
			prev.Index = call.Index
			prev.Arguments += call.Arguments
			acc[key] = prev
		}
	}
	for _, call := range acc {
		calls = append(calls, call)
	}
	return usage, calls
}

func mergeUsage(dst, src *coreruntime.TokenUsage) {
	if src.InputTokens != 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens != 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheRead != 0 {
		dst.CacheRead = src.CacheRead
	}
	if src.CacheWrite != 0 {
		dst.CacheWrite = src.CacheWrite
	}
	if src.Reasoning != 0 {
		dst.Reasoning = src.Reasoning
	}
	if src.AudioInput != 0 {
		dst.AudioInput = src.AudioInput
	}
	if src.AudioOutput != 0 {
		dst.AudioOutput = src.AudioOutput
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
}

func parseJSONToolCalls(body []byte) []runtime.ToolCall {
	var calls []runtime.ToolCall

	// Chat Completions non-stream and stream deltas.
	for _, choice := range gjson.GetBytes(body, "choices").Array() {
		for _, prefix := range []string{"message.tool_calls", "delta.tool_calls"} {
			for _, tc := range choice.Get(prefix).Array() {
				calls = append(calls, runtime.ToolCall{
					ID:        tc.Get("id").String(),
					Type:      defaultString(tc.Get("type").String(), "function"),
					Name:      tc.Get("function.name").String(),
					Arguments: tc.Get("function.arguments").String(),
					Index:     int(tc.Get("index").Int()),
				})
			}
		}
	}

	// Responses API function calls.
	for _, item := range gjson.GetBytes(body, "output").Array() {
		if isResponseToolCall(item) {
			calls = append(calls, responseToolCall(item, len(calls)))
		}
	}
	if item := gjson.GetBytes(body, "item"); item.Exists() && isResponseToolCall(item) {
		calls = append(calls, responseToolCall(item, len(calls)))
	}
	if delta := gjson.GetBytes(body, "delta"); delta.Exists() {
		name := gjson.GetBytes(body, "name").String()
		calls = append(calls, runtime.ToolCall{
			ID:        firstString(gjson.GetBytes(body, "call_id"), gjson.GetBytes(body, "item_id")),
			Type:      "function",
			Name:      name,
			Arguments: delta.String(),
			Index:     int(gjson.GetBytes(body, "output_index").Int()),
		})
	}

	return calls
}

func isResponseToolCall(item gjson.Result) bool {
	t := item.Get("type").String()
	return t == "function_call" || t == "tool_call" || strings.HasSuffix(t, ".function_call")
}

func responseToolCall(item gjson.Result, fallbackIndex int) runtime.ToolCall {
	idx := fallbackIndex
	if item.Get("index").Exists() {
		idx = int(item.Get("index").Int())
	}
	return runtime.ToolCall{
		ID:        firstString(item.Get("call_id"), item.Get("id")),
		Type:      defaultString(item.Get("type").String(), "function_call"),
		Name:      item.Get("name").String(),
		Arguments: item.Get("arguments").String(),
		Index:     idx,
	}
}

func observedToolSpans(calls []runtime.ToolCall) []coreruntime.ObservedSpan {
	spans := make([]coreruntime.ObservedSpan, 0, len(calls))
	for _, call := range calls {
		if call.Name == "" && call.ID == "" {
			continue
		}
		input := []byte(call.Arguments)
		attrs := map[string]any{
			"tool_call.id":    call.ID,
			"tool_call.name":  call.Name,
			"tool_call.type":  call.Type,
			"tool_call.index": call.Index,
		}
		spans = append(spans, coreruntime.ObservedSpan{
			Name:      "tool." + defaultString(call.Name, call.ID),
			Type:      coreruntime.TypeTool,
			Connector: "tool_call",
			Method:    defaultString(call.Type, "function"),
			Input:     &input,
			Attrs:     attrs,
		})
	}
	return spans
}

func callKey(call runtime.ToolCall) string {
	if call.ID != "" {
		return call.ID
	}
	return call.Name
}

func firstInt(values ...gjson.Result) int {
	for _, v := range values {
		if v.Exists() {
			return int(v.Int())
		}
	}
	return 0
}

func firstString(values ...gjson.Result) string {
	for _, v := range values {
		if v.Exists() && v.String() != "" {
			return v.String()
		}
	}
	return ""
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
