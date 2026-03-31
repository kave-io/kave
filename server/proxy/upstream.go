package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// UpstreamClient handles forwarding requests to upstream LLM providers.
type UpstreamClient struct {
	baseURLs map[string]string // "openai" -> "https://api.openai.com"
	clients  map[string]*http.Client
}

// NewUpstreamClient creates a new upstream client with default LLM provider URLs.
func NewUpstreamClient() *UpstreamClient {
	return &UpstreamClient{
		baseURLs: map[string]string{
			"openai":    "https://api.openai.com",
			"anthropic": "https://api.anthropic.com",
			"ollama":    "http://localhost:11434",
		},
		clients: map[string]*http.Client{
			"openai":    &http.Client{},
			"anthropic": &http.Client{},
			"ollama":    &http.Client{},
		},
	}
}

// Forward sends a request to the upstream provider and returns the response.
// It injects the real API key from credentials and extracts token usage.
func (u *UpstreamClient) Forward(req *http.Request, connector string, apiKey string) (*http.Response, error) {
	baseURL, ok := u.baseURLs[connector]
	if !ok {
		return nil, fmt.Errorf("unknown connector: %s", connector)
	}

	client, ok := u.clients[connector]
	if !ok {
		client = &http.Client{}
	}

	// Build upstream URL from path
	upstreamURL := baseURL + req.RequestURI

	// Create upstream request with same body
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	upstreamReq, err := http.NewRequest(req.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	// Copy headers
	upstreamReq.Header = req.Header.Clone()

	// Inject real API key, remove Kave token
	upstreamReq.Header.Del("Authorization")
	switch connector {
	case "openai":
		upstreamReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	case "anthropic":
		upstreamReq.Header.Set("X-API-Key", apiKey)
		upstreamReq.Header.Set("Anthropic-Version", "2023-06-01")
	}

	// Forward request
	resp, err := client.Do(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return resp, nil
}

// ExtractTokenUsage parses token usage from the response body.
// Returns a pointer that can be converted to *intercept.TokenUsage
func ExtractTokenUsage(body []byte, connector string) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	usage := map[string]interface{}{}

	switch connector {
	case "openai":
		if u, ok := data["usage"].(map[string]interface{}); ok {
			if pt, ok := u["prompt_tokens"].(float64); ok {
				usage["InputTokens"] = int(pt)
			}
			if ct, ok := u["completion_tokens"].(float64); ok {
				usage["OutputTokens"] = int(ct)
			}
		}
		if model, ok := data["model"].(string); ok {
			usage["Model"] = model
		}

	case "anthropic":
		if u, ok := data["usage"].(map[string]interface{}); ok {
			if it, ok := u["input_tokens"].(float64); ok {
				usage["InputTokens"] = int(it)
			}
			if ot, ok := u["output_tokens"].(float64); ok {
				usage["OutputTokens"] = int(ot)
			}
		}
		if model, ok := data["model"].(string); ok {
			usage["Model"] = model
		}

	case "ollama":
		if it, ok := data["prompt_eval_count"].(float64); ok {
			usage["InputTokens"] = int(it)
		}
		if ot, ok := data["eval_count"].(float64); ok {
			usage["OutputTokens"] = int(ot)
		}
		if model, ok := data["model"].(string); ok {
			usage["Model"] = model
		}
	}

	return usage
}
