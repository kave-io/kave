package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	base       string
	http       *http.Client
	defaultTTL KeepAlive
}

// base should be "http://localhost:11434" — no trailing slash.
func New(base string, timeout time.Duration) *Client {
	return &Client{
		base:       base,
		http:       &http.Client{Timeout: timeout},
		defaultTTL: KeepAlive5m,
	}
}

// WithDefaultTTL overrides the default keep_alive for Chat/Generate calls.
func (c *Client) WithDefaultTTL(ka KeepAlive) *Client {
	c.defaultTTL = ka
	return c
}

// ── Health ────────────────────────────────────────────────────────────────────

// Ping checks if Ollama is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ── Model management ──────────────────────────────────────────────────────────

// LoadModel wakes a model and keeps it in VRAM indefinitely.
func (c *Client) LoadModel(ctx context.Context, model string) error {
	req := GenerateRequest{
		Model:     model,
		Prompt:    "",
		KeepAlive: KeepAliveForever,
		Stream:    BoolPtr(false),
	}
	_, err := c.generate(ctx, req)
	if err != nil {
		return fmt.Errorf("ollama: load model %q: %w", model, err)
	}
	return nil
}

// UnloadModel sends an eviction request immediately.
func (c *Client) unloadModel(ctx context.Context, model string) error {
	req := GenerateRequest{
		Model:     model,
		Prompt:    "",
		KeepAlive: KeepAliveNone,
		Stream:    BoolPtr(false),
	}
	_, err := c.generate(ctx, req)
	if err != nil {
		return fmt.Errorf("ollama: unload model %q: %w", model, err)
	}
	return nil
}

// ForceUnload sends an eviction request and actively polls until the model is cleared from VRAM.
func (c *Client) ForceUnload(ctx context.Context, model string) error {
	if err := c.unloadModel(ctx, model); err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("ollama: timeout waiting for VRAM eviction of %q: %w", model, ctx.Err())
		case <-ticker.C:
			running, err := c.IsRunning(ctx, model)
			if err != nil {
				return err
			}
			if !running {
				return nil
			}
		}
	}
}

// ListRunning returns all models currently loaded in VRAM.
func (c *Client) ListRunning(ctx context.Context) ([]RunningModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	var result ListRunningResponse
	if err := c.do(req, &result); err != nil {
		return nil, fmt.Errorf("ollama: list running: %w", err)
	}
	return result.Models, nil
}

// ListModels returns all locally available models.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	var result ListResponse
	if err := c.do(req, &result); err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	return result.Models, nil
}

// IsRunning reports whether a specific model is currently in VRAM.
func (c *Client) IsRunning(ctx context.Context, model string) (bool, error) {
	running, err := c.ListRunning(ctx)
	if err != nil {
		return false, err
	}
	for _, m := range running {
		if m.Name == model || m.Model == model {
			return true, nil
		}
	}
	return false, nil
}

// ── Chat ──────────────────────────────────────────────────────────────────────

// Chat sends a multi-turn chat request.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.KeepAlive == "" {
		req.KeepAlive = c.defaultTTL
	}
	req.Stream = BoolPtr(false)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var result ChatResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("ollama: chat: %w", err)
	}
	return &result, nil
}

// ChatOneShot sends a chat request and evicts the model immediately after.
func (c *Client) ChatOneShot(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.KeepAlive = KeepAliveNone
	return c.Chat(ctx, req)
}

// ChatStreamChan returns a channel that streams the response token by token.
func (c *Client) ChatStreamChan(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	if req.KeepAlive == "" {
		req.KeepAlive = c.defaultTTL
	}
	req.Stream = BoolPtr(true)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: chat stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: chat stream: status %d", resp.StatusCode)
	}

	outChan := make(chan StreamChunk)

	go func() {
		defer resp.Body.Close()
		defer close(outChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk ChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				outChan <- StreamChunk{Error: fmt.Errorf("ollama: stream decode: %w", err)}
				return
			}

			select {
			case <-ctx.Done():
				outChan <- StreamChunk{Error: ctx.Err()}
				return
			case outChan <- StreamChunk{Content: chunk.Message.Content, Done: chunk.Done}:
			}

			if chunk.Done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			outChan <- StreamChunk{Error: err}
		}
	}()

	return outChan, nil
}

// ── Generate (raw, non-chat) ──────────────────────────────────────────────────

func (c *Client) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if req.KeepAlive == "" {
		req.KeepAlive = c.defaultTTL
	}
	req.Stream = BoolPtr(false)
	return c.generate(ctx, req)
}

func (c *Client) GenerateOneShot(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	req.KeepAlive = KeepAliveNone
	req.Stream = BoolPtr(false)
	return c.generate(ctx, req)
}

// GenerateStreamChan returns a channel that streams a raw completion.
func (c *Client) GenerateStreamChan(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if req.KeepAlive == "" {
		req.KeepAlive = c.defaultTTL
	}
	req.Stream = BoolPtr(true)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: generate stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: generate stream: status %d", resp.StatusCode)
	}

	outChan := make(chan StreamChunk)

	go func() {
		defer resp.Body.Close()
		defer close(outChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk GenerateResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				outChan <- StreamChunk{Error: fmt.Errorf("ollama: stream decode: %w", err)}
				return
			}

			select {
			case <-ctx.Done():
				outChan <- StreamChunk{Error: ctx.Err()}
				return
			case outChan <- StreamChunk{Content: chunk.Response, Done: chunk.Done}:
			}

			if chunk.Done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			outChan <- StreamChunk{Error: err}
		}
	}()

	return outChan, nil
}

// ── Embed ─────────────────────────────────────────────────────────────────────

// Embed generates embeddings for a batch of texts.
// Embed generates embeddings for a batch of texts.
// By default the model is kept in VRAM after the call.
// Pass EvictAfter() to unload immediately — use for isolated one-shot calls.
// For batch ingestion, call UnloadModel or ForceUnload explicitly when done.
func (c *Client) Embed(ctx context.Context, model string, texts []string, prefix EmbedPrefix, opts ...EmbedOption) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	cfg := &embedConfig{evictAfter: false} // default: keep alive
	for _, o := range opts {
		o(cfg)
	}

	inputs := make([]string, len(texts))
	for i, t := range texts {
		if prefix != PrefixNone {
			inputs[i] = string(prefix) + t
		} else {
			inputs[i] = t
		}
	}

	body, err := json.Marshal(EmbedRequest{Model: model, Input: inputs})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var result EmbedResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("ollama: embed: %w", err)
	}

	if cfg.evictAfter {
		go func() {
			unloadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.ForceUnload(unloadCtx, model)
		}()
	}

	return result.Embeddings, nil
}

// EmbedOne embeds a single text.
func (c *Client) EmbedOne(ctx context.Context, model, text string, prefix EmbedPrefix, opts ...EmbedOption) ([]float32, error) {
	results, err := c.Embed(ctx, model, []string{text}, prefix, opts...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("ollama: embed returned no results")
	}
	return results[0], nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var result GenerateResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("ollama: generate: %w", err)
	}
	return &result, nil
}

func (c *Client) do(req *http.Request, dst any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}
