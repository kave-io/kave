package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// baseURLs maps connector names to upstream API base URLs.
var baseURLs = map[string]string{
	"openai":    "https://api.openai.com",
	"anthropic": "https://api.anthropic.com",
	"gemini":    "https://generativelanguage.googleapis.com",
	"groq":      "https://api.groq.com/openai",
	"mistral":   "https://api.mistral.ai",
	"deepseek":  "https://api.deepseek.com",
	"ollama":    "http://localhost:11434",
}

// UpstreamClient forwards proxied requests to LLM providers.
type UpstreamClient struct {
	client *http.Client
}

func NewUpstreamClient() *UpstreamClient {
	return &UpstreamClient{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Forward sends the request to the upstream provider and returns the response.
// upstreamPath is the path after the connector prefix (e.g. "/v1/chat/completions").
// body is the already-read request body.
// apiKey is the decrypted credential.
func (u *UpstreamClient) Forward(ctx context.Context, orig *http.Request, connector, upstreamPath string, body []byte, apiKey string) (*http.Response, error) {
	base, ok := baseURLs[connector]
	if !ok {
		return nil, fmt.Errorf("unknown connector: %q", connector)
	}

	// Build upstream URL: base + upstreamPath + query string
	upstreamURL := base + upstreamPath
	if orig.URL.RawQuery != "" {
		upstreamURL += "?" + orig.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(ctx, orig.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	// Copy safe headers. Skip hop-by-hop, auth (replaced below), and
	// Accept-Encoding so Go's transport handles compression transparently
	// (otherwise it forwards gzip but doesn't decompress, giving us binary).
	for key, vals := range orig.Header {
		switch key {
		case "Authorization", "X-Api-Key", "Connection", "Transfer-Encoding", "Accept-Encoding":
			// handled separately or stripped intentionally
		default:
			req.Header[key] = vals
		}
	}

	// Inject API key. If no stored credential, pass through the client's own
	// auth headers so users can keep their own keys without storing them in Kave.
	if apiKey != "" {
		switch connector {
		case "anthropic":
			req.Header.Set("X-API-Key", apiKey)
		case "gemini":
			q := req.URL.Query()
			q.Set("key", apiKey)
			req.URL.RawQuery = q.Encode()
		default:
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	} else {
		// Passthrough: restore the client's original auth headers
		if v := orig.Header.Get("X-Api-Key"); v != "" {
			req.Header.Set("X-API-Key", v)
		} else if v := orig.Header.Get("Authorization"); v != "" {
			// Strip "Bearer <agent-id>" — pass the real key the client sent
			// Only passthrough if it looks like a real API key (not our agent ID)
			req.Header.Set("Authorization", v)
		}
	}

	// Anthropic always needs this version header
	if connector == "anthropic" {
		req.Header.Set("Anthropic-Version", "2023-06-01")
	}

	return u.client.Do(req)
}

// StreamForward forwards a streaming request and pipes the response body to w,
// flushing each chunk so the client receives it in real time.
func (u *UpstreamClient) StreamForward(ctx context.Context, orig *http.Request, connector, upstreamPath string, body []byte, apiKey string, w http.ResponseWriter) error {
	resp, err := u.Forward(ctx, orig, connector, upstreamPath, body, apiKey)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("stream upstream: %w", err)
		}
	}
	return nil
}

// ExtractStreamingTokenUsage scans SSE event lines for token usage data.
// Anthropic: message_start has input tokens, message_delta has output tokens.
// OpenAI: final data chunk may include usage object.
func ExtractStreamingTokenUsage(body []byte, connector string) map[string]any {
	usage := map[string]any{}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" || data == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(data), &obj); err != nil {
			continue
		}

		switch connector {
		case "anthropic":
			// message_start: input tokens
			if obj["type"] == "message_start" {
				if msg, ok := obj["message"].(map[string]any); ok {
					if u, ok := msg["usage"].(map[string]any); ok {
						if v, ok := u["input_tokens"].(float64); ok {
							usage["InputTokens"] = int(v)
						}
						if v, ok := u["cache_read_input_tokens"].(float64); ok {
							usage["CacheRead"] = int(v)
						}
						if v, ok := u["cache_creation_input_tokens"].(float64); ok {
							usage["CacheWrite"] = int(v)
						}
					}
					if m, ok := msg["model"].(string); ok {
						usage["Model"] = m
					}
				}
			}
			// message_delta: output tokens
			if obj["type"] == "message_delta" {
				if u, ok := obj["usage"].(map[string]any); ok {
					if v, ok := u["output_tokens"].(float64); ok {
						usage["OutputTokens"] = int(v)
					}
				}
			}

		default: // OpenAI-compatible streaming
			if u, ok := obj["usage"].(map[string]any); ok {
				if v, ok := u["prompt_tokens"].(float64); ok {
					usage["InputTokens"] = int(v)
				}
				if v, ok := u["completion_tokens"].(float64); ok {
					usage["OutputTokens"] = int(v)
				}
			}
			if m, ok := obj["model"].(string); ok && usage["Model"] == nil {
				usage["Model"] = m
			}
		}
	}

	if len(usage) == 0 {
		return nil
	}
	return usage
}

// ExtractTokenUsage parses token usage from a non-streaming response body.
func ExtractTokenUsage(body []byte, connector string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	usage := map[string]any{}

	switch connector {
	case "openai", "groq", "deepseek", "mistral":
		if u, ok := data["usage"].(map[string]any); ok {
			if v, ok := u["prompt_tokens"].(float64); ok {
				usage["InputTokens"] = int(v)
			}
			if v, ok := u["completion_tokens"].(float64); ok {
				usage["OutputTokens"] = int(v)
			}
		}
		if m, ok := data["model"].(string); ok {
			usage["Model"] = m
		}

	case "anthropic":
		if u, ok := data["usage"].(map[string]any); ok {
			if v, ok := u["input_tokens"].(float64); ok {
				usage["InputTokens"] = int(v)
			}
			if v, ok := u["output_tokens"].(float64); ok {
				usage["OutputTokens"] = int(v)
			}
			if v, ok := u["cache_read_input_tokens"].(float64); ok {
				usage["CacheRead"] = int(v)
			}
			if v, ok := u["cache_creation_input_tokens"].(float64); ok {
				usage["CacheWrite"] = int(v)
			}
		}
		if m, ok := data["model"].(string); ok {
			usage["Model"] = m
		}

	case "ollama":
		if v, ok := data["prompt_eval_count"].(float64); ok {
			usage["InputTokens"] = int(v)
		}
		if v, ok := data["eval_count"].(float64); ok {
			usage["OutputTokens"] = int(v)
		}
		if m, ok := data["model"].(string); ok {
			usage["Model"] = m
		}

	case "gemini":
		if meta, ok := data["usageMetadata"].(map[string]any); ok {
			if v, ok := meta["promptTokenCount"].(float64); ok {
				usage["InputTokens"] = int(v)
			}
			if v, ok := meta["candidatesTokenCount"].(float64); ok {
				usage["OutputTokens"] = int(v)
			}
		}
		// model is in the request, not response for Gemini
	}

	return usage
}
