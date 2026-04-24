package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *HTTPClient) Stream(ctx context.Context, path string, query url.Values) (io.ReadCloser, error) {
	if c == nil {
		return nil, fmt.Errorf("runtime client missing")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if token, err := c.sessionToken(); err == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return nil, decodeCommandError(raw, resp.StatusCode)
	}
	return resp.Body, nil
}

func ReadJSONL[T any](r io.Reader, fn func(T) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame T
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			return err
		}
		if fn != nil {
			if err := fn(frame); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
