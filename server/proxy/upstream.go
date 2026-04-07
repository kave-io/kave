package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	// Copy safe headers, skip hop-by-hop and Kave's own Authorization
	for key, vals := range orig.Header {
		switch key {
		case "Authorization", "X-Api-Key", "Connection", "Transfer-Encoding":
			// replaced below
		default:
			req.Header[key] = vals
		}
	}

	// Inject the real API key in the provider's expected format
	switch connector {
	case "anthropic":
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
	case "gemini":
		// Gemini uses ?key= query param
		q := req.URL.Query()
		q.Set("key", apiKey)
		req.URL.RawQuery = q.Encode()
	default:
		// OpenAI-compatible: Bearer token
		req.Header.Set("Authorization", "Bearer "+apiKey)
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
