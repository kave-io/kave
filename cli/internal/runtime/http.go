package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/kave-io/kave/cli/internal/contract"
	"github.com/kave-io/kave/cli/internal/output"
	corekeyring "github.com/kave-io/kave/core/pkg/keyring"
)

const sessionKeyringService = "kave"

type HTTPClient struct {
	BaseURL    string
	HTTP       *http.Client
	SessionKey string
}


func resolveBaseURL(rt *Runtime) string {
	if rt == nil || rt.Resolution == nil {
		return "http://127.0.0.1:8080"
	}
	if server := strings.TrimSpace(rt.Resolution.ActiveServer()); server != "" {
		return normalizeURL(server)
	}
	return "http://127.0.0.1:8080"
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return strings.TrimRight(raw, "/")
	}
	return "http://" + strings.TrimRight(raw, "/")
}

func sessionAccount(baseURL string) string {
	sum := sha256.Sum256([]byte(baseURL))
	return fmt.Sprintf("session:%x", sum[:8])
}

func (c *HTTPClient) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out)
}

func (c *HTTPClient) Delete(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodDelete, path, query, nil, out)
}

func (c *HTTPClient) Post(ctx context.Context, path string, query url.Values, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, query, body, out)
}

func (c *HTTPClient) Put(ctx context.Context, path string, query url.Values, body any, out any) error {
	return c.doJSON(ctx, http.MethodPut, path, query, body, out)
}

func (c *HTTPClient) Patch(ctx context.Context, path string, query url.Values, body any, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, query, body, out)
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if c == nil {
		return fmt.Errorf("runtime client missing")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token, err := c.sessionToken(); err == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeCommandError(raw, resp.StatusCode)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}

	var envelope contract.SuccessEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if out != nil && envelope.Data != nil {
		if err := assignEnvelopeData(out, envelope.Data); err != nil {
			return err
		}
	}
	return nil
}

func assignEnvelopeData(out any, data any) error {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("output must be a non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}

	field := elem.FieldByName("Data")
	if field.IsValid() && field.CanSet() {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		target := reflect.New(field.Type())
		if err := json.Unmarshal(raw, target.Interface()); err != nil {
			return err
		}
		field.Set(target.Elem())
		return nil
	}

	field = elem.FieldByName("Items")
	if field.IsValid() && field.CanSet() {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		target := reflect.New(field.Type())
		if err := json.Unmarshal(raw, target.Interface()); err != nil {
			return err
		}
		field.Set(target.Elem())
		return nil
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func decodeCommandError(raw []byte, statusCode int) error {
	var env contract.ErrorEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Error.Message != "" {
		exit := classifyExitCode(env.Error.Code, statusCode)
		return &output.CommandError{
			Code:    env.Error.Code,
			Message: env.Error.Message,
			Details: env.Error.Details,
			Exit:    exit,
		}
	}
	if len(raw) == 0 {
		return &output.CommandError{Code: "command.failed", Message: http.StatusText(statusCode), Exit: 1}
	}
	return &output.CommandError{Code: "command.failed", Message: strings.TrimSpace(string(raw)), Exit: 1}
}

func classifyExitCode(code string, statusCode int) int {
	switch {
	case strings.HasPrefix(code, "auth."):
		return 77
	case code == "gateway.policy_blocked":
		return 75
	case code == "gateway.budget_exceeded":
		return 74
	case statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity:
		return 64
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return 77
	default:
		return 1
	}
}

func (c *HTTPClient) sessionToken() (string, error) {
	// Env override: lets headless/CI use a PAT without the OS keyring.
	if tok := strings.TrimSpace(os.Getenv("KAVE_TOKEN")); tok != "" {
		return tok, nil
	}
	if c == nil || c.SessionKey == "" {
		return "", nil
	}
	return corekeyring.Get(sessionKeyringService, c.SessionKey)
}

func (c *HTTPClient) SaveSessionToken(token string) error {
	if c == nil {
		return fmt.Errorf("runtime client missing")
	}
	return corekeyring.Set(sessionKeyringService, c.SessionKey, token)
}

func (c *HTTPClient) ClearSessionToken() error {
	if c == nil {
		return fmt.Errorf("runtime client missing")
	}
	return corekeyring.Delete(sessionKeyringService, c.SessionKey)
}
