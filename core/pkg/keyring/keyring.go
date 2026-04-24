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

// Set stores a secret in the OS keyring. It returns ErrKeyringUnavailable if
// the system keyring cannot be used and plaintext fallback is not permitted.
func Set(service, account, value string) error {
	if os.Getenv("KAVE_KEYRING_DISABLED") != "1" {
		if err := osring.Set(service, account, value); err == nil {
			return nil
		}
	}
	return plaintextSecretFallback(service, account, value)
}

// Get retrieves a secret from the OS keyring.
func Get(service, account string) (string, error) {
	if os.Getenv("KAVE_KEYRING_DISABLED") != "1" {
		if value, err := osring.Get(service, account); err == nil {
			return value, nil
		} else if !errors.Is(err, osring.ErrNotFound) {
			if fallback, fallbackErr := plaintextSecretRead(service, account); fallbackErr == nil {
				return fallback, nil
			}
			return "", fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
		}
	}
	return plaintextSecretRead(service, account)
}

// Delete removes a secret from the OS keyring.
func Delete(service, account string) error {
	if os.Getenv("KAVE_KEYRING_DISABLED") != "1" {
		if err := osring.Delete(service, account); err == nil || errors.Is(err, osring.ErrNotFound) {
			_ = plaintextSecretDelete(service, account)
			return nil
		}
	}
	return plaintextSecretDelete(service, account)
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

func plaintextSecretPath(service, account string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ErrKeyringUnavailable
	}
	dir := filepath.Join(home, ".kave", "keyring")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("%w: mkdir: %v", ErrKeyringUnavailable, err)
	}
	name := base64.RawURLEncoding.EncodeToString([]byte(service + "\x00" + account))
	return filepath.Join(dir, name), nil
}

func plaintextSecretFallback(service, account, value string) error {
	path, err := plaintextSecretPath(service, account)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0o600)
}

func plaintextSecretRead(service, account string) (string, error) {
	path, err := plaintextSecretPath(service, account)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func plaintextSecretDelete(service, account string) error {
	path, err := plaintextSecretPath(service, account)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
