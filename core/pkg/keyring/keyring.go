package keyring

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrKeyringUnavailable = errors.New("keyring unavailable")

// GetOrCreateMasterKey returns a 32-byte master key.
// This is a thin local-development fallback that persists under ~/.kave/.
func GetOrCreateMasterKey(_ context.Context) ([]byte, error) {
	if os.Getenv("KAVE_KEYRING_DISABLED") == "1" {
		return nil, ErrKeyringUnavailable
	}
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
