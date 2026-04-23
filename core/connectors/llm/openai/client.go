package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBase = "https://api.openai.com/v1"

type Client struct {
	base   string
	apiKey string
	org    string
	http   *http.Client
}

func New(apiKey string, timeout time.Duration) *Client {
	return &Client{
		base:   defaultBase,
		apiKey: apiKey,
		http:   &http.Client{Timeout: timeout},
	}
}

func (c *Client) WithOrg(org string) *Client {
	c.org = org
	return c
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = nil
	req.StreamOptions = nil

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("openai: chat: %w", err)
	}
	return &result, nil
}

// ChatStream sends a streaming chat completion request and returns a channel of chunks.
// The channel is always closed when the stream ends. Usage is included in the final chunk
// when stream_options.include_usage is set (which this method does automatically).
func (c *Client) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	stream := true
	req.Stream = &stream
	req.StreamOptions = &StreamOptions{IncludeUsage: true}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: chat stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.decodeError(resp)
	}

	ch := make(chan StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || line == "data: [DONE]" {
				continue
			}
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}

			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case ch <- chunk:
			}
		}
	}()

	return ch, nil
}

// Embed creates embeddings for the given input.
func (c *Client) Embed(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/embeddings", body)
	if err != nil {
		return nil, err
	}

	var result EmbedResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("openai: embed: %w", err)
	}
	return &result, nil
}

// Models lists available models.
func (c *Client) Models(ctx context.Context) (*ModelList, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}

	var result ModelList
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("openai: models: %w", err)
	}
	return &result, nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if c.org != "" {
		req.Header.Set("OpenAI-Organization", c.org)
	}
	return req, nil
}

func (c *Client) do(req *http.Request, dst any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.decodeError(resp)
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *Client) decodeError(resp *http.Response) error {
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("openai: status %d", resp.StatusCode)
	}
	return fmt.Errorf("openai: %s (%s)", errResp.Error.Message, errResp.Error.Type)
}
