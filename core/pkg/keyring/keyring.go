package keyring

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	osring "github.com/zalando/go-keyring"
)

var ErrKeyringUnavailable = errors.New("keyring unavailable")

const (
	service = "kave"
	account = "master-key"
)

// GetOrCreateMasterKey returns a 32-byte master key from the OS keyring.
// If the OS keyring is unavailable and KAVE_ALLOW_PLAINTEXT_KEY=1, falls back
// to a 0600 file at ~/.kave/master.key (dev only).
func GetOrCreateMasterKey(_ context.Context) ([]byte, error) {
	if os.Getenv("KAVE_KEYRING_DISABLED") == "1" {
		return nil, ErrKeyringUnavailable
	}

	if s, err := osring.Get(service, account); err == nil {
		key, decErr := base64.StdEncoding.DecodeString(s)
		if decErr == nil && len(key) == 32 {
			return key, nil
		}
	} else if !errors.Is(err, osring.ErrNotFound) {
		// Keyring reachable but errored, or unreachable — try fallback below.
		if os.Getenv("KAVE_ALLOW_PLAINTEXT_KEY") != "1" {
			return nil, fmt.Errorf("%w: %v (set KAVE_ALLOW_PLAINTEXT_KEY=1 for dev fallback)", ErrKeyringUnavailable, err)
		}
		return plaintextFallback()
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: generate: %v", ErrKeyringUnavailable, err)
	}
	if err := osring.Set(service, account, base64.StdEncoding.EncodeToString(key)); err != nil {
		if os.Getenv("KAVE_ALLOW_PLAINTEXT_KEY") != "1" {
			return nil, fmt.Errorf("%w: store: %v (set KAVE_ALLOW_PLAINTEXT_KEY=1 for dev fallback)", ErrKeyringUnavailable, err)
		}
		return plaintextFallback()
	}
	return key, nil
}

func plaintextFallback() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, ErrKeyringUnavailable
	}
	dir := filepath.Join(home, ".kave")
	path := filepath.Join(dir, "master.key")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		out := make([]byte, 32)
		copy(out, b)
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: mkdir: %v", ErrKeyringUnavailable, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: generate: %v", ErrKeyringUnavailable, err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("%w: write: %v", ErrKeyringUnavailable, err)
	}
	return key, nil
}
