package credresolve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VaultResolver resolves secrets from a HashiCorp Vault KV v2 mount.
type VaultResolver struct {
	Addr   string
	Token  string
	Mount  string
	Client *http.Client
}

func (r *VaultResolver) Resolve(ctx context.Context, ref string) (string, error) {
	if r == nil || strings.TrimSpace(r.Addr) == "" || strings.TrimSpace(r.Mount) == "" {
		return "", ErrSourceDisabled
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	mount := strings.Trim(r.Mount, "/")
	secretRef := strings.Trim(ref, "/")
	endpoint := fmt.Sprintf("%s/v1/%s/%s", strings.TrimRight(r.Addr, "/"), mount, url.PathEscape(secretRef))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if r.Token != "" {
		req.Header.Set("X-Vault-Token", r.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("vault lookup failed: %s", strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data any `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		return "", err
	}

	if s := extractVaultValue(payload.Data); s != "" {
		return s, nil
	}
	return "", fmt.Errorf("vault secret %q missing value", ref)
}

func extractVaultValue(v any) string {
	switch m := v.(type) {
	case map[string]any:
		if inner, ok := m["data"].(map[string]any); ok {
			if val, ok := inner["value"].(string); ok && val != "" {
				return val
			}
			if val, ok := inner["secret"].(string); ok && val != "" {
				return val
			}
			for _, k := range []string{"data", "value", "secret"} {
				if s, ok := inner[k].(string); ok && s != "" {
					return s
				}
			}
		}
		for _, k := range []string{"value", "secret"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
