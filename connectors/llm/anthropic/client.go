package anthropic

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

const (
	defaultBase = "https://api.anthropic.com"
	apiVersion  = "2023-06-01"
)

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

func (c *Client) Messages(ctx context.Context, req MessagesRequest) (*MessagesResponse, error) {
	req.Stream = nil
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/v1/messages", body)
	if err != nil {
		return nil, err
	}
	var result MessagesResponse
	if err := c.do(httpReq, &result); err != nil {
		return nil, fmt.Errorf("anthropic: messages: %w", err)
	}
	return &result, nil
}

func (c *Client) MessagesStream(ctx context.Context, req MessagesRequest) (<-chan StreamChunk, error) {
	t := true
	req.Stream = &t
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/v1/messages", body)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: messages stream: %w", err)
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

			var ev streamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}

			switch ev.Type {
			case "content_block_delta":
				var evt contentBlockDeltaEvent
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					continue
				}
				if evt.Delta.Type == "text_delta" && evt.Delta.Text != "" {
					select {
					case <-ctx.Done():
						ch <- StreamChunk{Error: ctx.Err()}
						return
					case ch <- StreamChunk{Content: evt.Delta.Text}:
					}
				}
			case "message_stop":
				ch <- StreamChunk{Done: true}
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
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
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
		return fmt.Errorf("anthropic: status %d", resp.StatusCode)
	}
	return fmt.Errorf("anthropic: %s (%s)", e.Error.Message, e.Error.Type)
}
