// Package codexauth reads the ChatGPT-login credential that OpenAI's codex
// CLI maintains in ~/.codex/auth.json so the openai connector can forward
// codex requests to chatgpt.com/backend-api/codex/* on behalf of the user
// without storing or refreshing the token itself. Codex owns the file; we
// only read it.
package codexauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotConfigured indicates the codex auth file is absent or has no token.
// Callers should treat this as "ChatGPT-login passthrough is unavailable" and
// fall back to whatever credential the inbound caller supplied.
var ErrNotConfigured = errors.New("codex auth not configured")

type Auth struct {
	AccessToken string
	AccountID   string
}

// Load reads ~/.codex/auth.json (or $KAVE_CODEX_AUTH_PATH if set) and returns
// the access token and ChatGPT account id. Re-read on every call so codex's
// background token refresh is picked up automatically.
func Load() (*Auth, error) {
	path := strings.TrimSpace(os.Getenv("KAVE_CODEX_AUTH_PATH"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}
		path = filepath.Join(home, ".codex", "auth.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}
	var raw struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse codex auth: %w", err)
	}
	if raw.Tokens.AccessToken == "" {
		return nil, ErrNotConfigured
	}
	return &Auth{AccessToken: raw.Tokens.AccessToken, AccountID: raw.Tokens.AccountID}, nil
}
