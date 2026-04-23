package gemini

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

const defaultBase = "https://generativelanguage.googleapis.com/v1beta"

type Client struct {
	base   string
	apiKey string
	http   *http.Client
}

func New(apiKey string, timeout time.Duration) *Client {
	return &Client{
		base:   defaultBase,
		apiKey: apiKey,
		http:   &http.Client{Timeout: timeout},
	}
}

func (c *Client) Generate(ctx context.Context, model string, req GenerateRequest) (*GenerateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/models/"+model+":generateContent", body)
	if err != nil {
		return nil, err
	}
	var result GenerateResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("gemini: generate: %w", err)
	}
	return &result, nil
}

func (c *Client) GenerateStream(ctx context.Context, model string, req GenerateRequest) (<-chan StreamChunk, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/models/"+model+":streamGenerateContent?alt=sse", body)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: generate stream: %w", err)
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
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok || data == "" {
				continue
			}

			var chunk GenerateResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			var text string
			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				text = chunk.Candidates[0].Content.Parts[0].Text
			}

			done := len(chunk.Candidates) > 0 && chunk.Candidates[0].FinishReason != ""

			select {
			case <-ctx.Done():
				ch <- StreamChunk{Error: ctx.Err()}
				return
			case ch <- StreamChunk{Content: text, Done: done}:
			}

			if done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Error: err}
		}
	}()

	return ch, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
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
	var e ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return fmt.Errorf("gemini: status %d", resp.StatusCode)
	}
	return fmt.Errorf("gemini: %s (%s)", e.Error.Message, e.Error.Status)
}
