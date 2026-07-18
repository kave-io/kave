package v2

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// RawServiceKeyPrefix identifies Kave V2 machine credentials.
	RawServiceKeyPrefix = "kv2_"

	serviceKeyLookupBytes = 18
	serviceKeySecretBytes = 32
	serviceKeyLookupChars = 24
	serviceKeySecretChars = 43
)

// ServiceKeyMaterial is generated and retained by the credential recipient.
// Only LookupPrefix and SecretHash cross the control API; RawKey never does.
type ServiceKeyMaterial struct {
	LookupPrefix string
	SecretHash   [sha256.Size]byte
	RawKey       string `json:"-"`
}

// GenerateServiceKeyMaterial creates a canonical high-entropy Kave V2
// credential. Passing nil uses crypto/rand.Reader.
func GenerateServiceKeyMaterial(random io.Reader) (ServiceKeyMaterial, error) {
	if random == nil {
		random = rand.Reader
	}
	lookup := make([]byte, serviceKeyLookupBytes)
	if _, err := io.ReadFull(random, lookup); err != nil {
		return ServiceKeyMaterial{}, fmt.Errorf("kave v2: generate service-key lookup prefix: %w", err)
	}
	secret := make([]byte, serviceKeySecretBytes)
	if _, err := io.ReadFull(random, secret); err != nil {
		return ServiceKeyMaterial{}, fmt.Errorf("kave v2: generate service-key secret: %w", err)
	}
	defer clear(secret)

	lookupPrefix := base64.RawURLEncoding.EncodeToString(lookup)
	rawKey := RawServiceKeyPrefix + lookupPrefix + "." + base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(rawKey))
	return ServiceKeyMaterial{LookupPrefix: lookupPrefix, SecretHash: digest, RawKey: rawKey}, nil
}

// ParseServiceKeyMaterial validates a canonical raw credential and derives
// the one-way material accepted by the control plane.
func ParseServiceKeyMaterial(rawKey string) (ServiceKeyMaterial, error) {
	if rawKey == "" || strings.TrimSpace(rawKey) != rawKey {
		return ServiceKeyMaterial{}, errors.New("kave v2: invalid service key")
	}
	body, ok := strings.CutPrefix(rawKey, RawServiceKeyPrefix)
	if !ok {
		return ServiceKeyMaterial{}, errors.New("kave v2: invalid service key")
	}
	lookupPrefix, secret, ok := strings.Cut(body, ".")
	if !ok || !validServiceKeyLookupPrefix(lookupPrefix) || !validEncodedServiceKeyPart(secret, serviceKeySecretBytes, serviceKeySecretChars) {
		return ServiceKeyMaterial{}, errors.New("kave v2: invalid service key")
	}
	digest := sha256.Sum256([]byte(rawKey))
	return ServiceKeyMaterial{LookupPrefix: lookupPrefix, SecretHash: digest, RawKey: rawKey}, nil
}

// ValidateServiceKeyVerifier validates the only credential material accepted
// by service-key issuance. It deliberately does not accept a raw credential.
func ValidateServiceKeyVerifier(lookupPrefix string, secretHash []byte) error {
	if err := ValidateServiceKeyLookupPrefix(lookupPrefix); err != nil {
		return err
	}
	if len(secretHash) != sha256.Size {
		return invalid("service_key.secret_hash", "must be a 32-byte SHA-256 verifier")
	}
	var zero [sha256.Size]byte
	if subtle.ConstantTimeCompare(secretHash, zero[:]) == 1 {
		return invalid("service_key.secret_hash", "must not be all zeroes")
	}
	return nil
}

// ValidateServiceKeyLookupPrefix accepts only the canonical non-secret lookup
// component generated for a Kave V2 service key.
func ValidateServiceKeyLookupPrefix(lookupPrefix string) error {
	if !validServiceKeyLookupPrefix(lookupPrefix) {
		return invalid("service_key.lookup_prefix", "must be canonical unpadded base64url encoding of 18 bytes")
	}
	return nil
}

func validServiceKeyLookupPrefix(value string) bool {
	return validEncodedServiceKeyPart(value, serviceKeyLookupBytes, serviceKeyLookupChars)
}

func validEncodedServiceKeyPart(value string, decodedBytes, encodedChars int) bool {
	if len(value) != encodedChars {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != decodedBytes {
		return false
	}
	return base64.RawURLEncoding.EncodeToString(decoded) == value
}
