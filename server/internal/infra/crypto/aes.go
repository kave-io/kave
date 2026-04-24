// Package crypto stays under server/internal/infra because it is a server-side
// secret-handling wrapper, not a generic cryptography API. Its callers live in
// the server credential path and the implementation is intentionally local to
// that trust boundary.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey         = errors.New("encryption key must be 32 bytes")
	ErrCiphertextTooShort = errors.New("ciphertext too short")
)

type AES struct {
	gcm cipher.AEAD
}

// NewAES creates an AES-256-GCM instance.
func NewAES(key []byte) (*AES, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &AES{gcm: gcm}, nil
}

// Encrypt returns raw bytes in format:
// [nonce || ciphertext || auth_tag]
func (a *AES) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := a.gcm.Seal(nil, nonce, plaintext, aad)

	return append(nonce, ciphertext...), nil
}

// Decrypt expects:
// [nonce || ciphertext || auth_tag]
func (a *AES) Decrypt(data []byte, aad []byte) ([]byte, error) {
	nonceSize := a.gcm.NonceSize()

	if len(data) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := a.gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// Hash returns SHA-256 hex digest of a value.
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// KeyFromHex decodes 64-char hex into 32-byte AES key.
func KeyFromHex(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return key, nil
}
