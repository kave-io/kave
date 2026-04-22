package authhash

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	saltSize = 16
	keyLen   = 32
)

// HashPassword hashes a password with argon2id.
func HashPassword(pw string) ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(pw), salt, 1, 64*1024, 4, keyLen)
	out := append([]byte("argon2id$"), salt...)
	out = append(out, hash...)
	return out, nil
}

// VerifyPassword compares an argon2id hash against a plaintext password.
func VerifyPassword(hash []byte, pw string) bool {
	if len(hash) != len("argon2id$")+saltSize+keyLen {
		return false
	}
	salt := hash[len("argon2id$") : len("argon2id$")+saltSize]
	want := hash[len("argon2id$")+saltSize:]
	got := argon2.IDKey([]byte(pw), salt, 1, 64*1024, 4, keyLen)
	return subtleCompare(got, want)
}

// HashToken returns the deterministic SHA-256 hash of a raw token.
func HashToken(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	out := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(out, sum[:])
	return out
}

// GenerateToken creates a raw token and its hash.
func GenerateToken(prefix string) (plain string, hash []byte, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	plain = prefix + base64.RawURLEncoding.EncodeToString(raw[:])
	return plain, HashToken(plain), nil
}

func subtleCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

var ErrInvalidTokenHash = errors.New("invalid token hash")
