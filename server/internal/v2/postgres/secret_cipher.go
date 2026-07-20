package postgres

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	localEnvelopeVersion = byte(1)
	dataKeySize          = 32
)

var ErrSecretDecrypt = errors.New("v2 postgres: cannot decrypt secret")

type SecretAAD struct {
	AccountID   string
	NamespaceID string
	Name        string
	Version     int64
}

type SecretCipher interface {
	Seal(context.Context, SecretAAD, []byte) (ciphertext []byte, wrappingKeyID string, err error)
	Open(context.Context, SecretAAD, []byte, string) ([]byte, error)
}

// SecretIdempotencyMAC binds write-only secret material to an idempotency
// record without persisting a dictionary-testable plaintext digest. The key
// must be independent from the AEAD master-key use through domain separation.
type SecretIdempotencyMAC interface {
	SecretIdempotencyMAC([]byte) (string, error)
}

// LocalEnvelopeCipher implements explicit-key envelope encryption for
// self-hosted Kave. A random data-encryption key protects each secret version;
// the configured master key only wraps that DEK. The database stores neither
// plaintext nor an unwrapped DEK.
type LocalEnvelopeCipher struct {
	masters        map[string]cipher.AEAD
	current        cipher.AEAD
	currentKeyID   string
	idempotencyKey [sha256.Size]byte
}

// LocalEnvelopeKeyring supports online master-key rotation. CurrentKey seals
// new versions; DecryptionKeys retain read access to older wrapping_key_id
// values. Rotating keyrings require a stable, separate IdempotencyKey so an
// encryption-key rotation cannot change prior secret-write request MACs.
type LocalEnvelopeKeyring struct {
	CurrentKey     string
	DecryptionKeys []string
	IdempotencyKey string
}

// NewLocalEnvelopeCipher accepts exactly 32 bytes encoded as 64 hexadecimal
// characters or standard/raw base64. An empty key is rejected rather than
// silently generating host-local key material.
func NewLocalEnvelopeCipher(encodedKey string) (*LocalEnvelopeCipher, error) {
	return NewLocalEnvelopeCipherKeyring(LocalEnvelopeKeyring{CurrentKey: encodedKey})
}

func NewLocalEnvelopeCipherKeyring(keyring LocalEnvelopeKeyring) (*LocalEnvelopeCipher, error) {
	current, currentKeyID, err := localMasterAEAD(keyring.CurrentKey)
	if err != nil {
		return nil, err
	}
	masters := map[string]cipher.AEAD{currentKeyID: current}
	for _, encoded := range keyring.DecryptionKeys {
		master, keyID, err := localMasterAEAD(encoded)
		if err != nil {
			return nil, fmt.Errorf("v2 postgres: configure decryption key: %w", err)
		}
		if _, exists := masters[keyID]; exists {
			continue
		}
		masters[keyID] = master
	}

	idempotencyEncoded := keyring.IdempotencyKey
	if idempotencyEncoded == "" {
		if len(keyring.DecryptionKeys) > 0 {
			return nil, errors.New("v2 postgres: rotating keyring requires an explicit stable idempotency key")
		}
		idempotencyEncoded = keyring.CurrentKey
	}
	idempotencyMaster, err := decodeMasterKey(idempotencyEncoded)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: decode secret idempotency key: %w", err)
	}
	derive := hmac.New(sha256.New, idempotencyMaster)
	_, _ = derive.Write([]byte("kave-v2-secret-idempotency-key\x00v1"))
	derived := derive.Sum(nil)
	var idempotencyKey [sha256.Size]byte
	copy(idempotencyKey[:], derived)
	clear(derived)
	clear(idempotencyMaster)
	return &LocalEnvelopeCipher{
		masters: masters, current: current, currentKeyID: currentKeyID,
		idempotencyKey: idempotencyKey,
	}, nil
}

func (c *LocalEnvelopeCipher) SecretIdempotencyMAC(payload []byte) (string, error) {
	if c == nil || c.current == nil {
		return "", errors.New("v2 postgres: secret idempotency authentication unavailable")
	}
	mac := hmac.New(sha256.New, c.idempotencyKey[:])
	_, _ = mac.Write([]byte("kave-v2-secret-idempotency-request\x00v1\x00"))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (c *LocalEnvelopeCipher) Seal(_ context.Context, aad SecretAAD, plaintext []byte) ([]byte, string, error) {
	if c == nil || c.current == nil || len(plaintext) == 0 {
		return nil, "", errors.New("v2 postgres: secret encryption unavailable")
	}
	dataKey := make([]byte, dataKeySize)
	if _, err := rand.Read(dataKey); err != nil {
		return nil, "", fmt.Errorf("v2 postgres: generate data key: %w", err)
	}
	defer clear(dataKey)
	dataBlock, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, "", fmt.Errorf("v2 postgres: create data cipher: %w", err)
	}
	dataAEAD, err := cipher.NewGCM(dataBlock)
	if err != nil {
		return nil, "", fmt.Errorf("v2 postgres: create data AEAD: %w", err)
	}

	wrapNonce := make([]byte, c.current.NonceSize())
	dataNonce := make([]byte, dataAEAD.NonceSize())
	if _, err := rand.Read(wrapNonce); err != nil {
		return nil, "", fmt.Errorf("v2 postgres: generate wrapping nonce: %w", err)
	}
	if _, err := rand.Read(dataNonce); err != nil {
		return nil, "", fmt.Errorf("v2 postgres: generate data nonce: %w", err)
	}
	authenticated := secretAAD(aad)
	wrappedKey := c.current.Seal(nil, wrapNonce, dataKey, append([]byte("kave-v2-wrap\x00"), authenticated...))
	sealedSecret := dataAEAD.Seal(nil, dataNonce, plaintext, authenticated)

	// Version and fixed-size sections make the format deterministic to parse
	// while AEAD authentication detects truncation, swapping, and tampering.
	blob := make([]byte, 0, 1+len(wrapNonce)+len(wrappedKey)+len(dataNonce)+len(sealedSecret))
	blob = append(blob, localEnvelopeVersion)
	blob = append(blob, wrapNonce...)
	blob = append(blob, wrappedKey...)
	blob = append(blob, dataNonce...)
	blob = append(blob, sealedSecret...)
	return blob, c.currentKeyID, nil
}

func (c *LocalEnvelopeCipher) Open(_ context.Context, aad SecretAAD, blob []byte, wrappingKeyID string) ([]byte, error) {
	if c == nil || c.masters == nil {
		return nil, ErrSecretDecrypt
	}
	master, exists := c.masters[wrappingKeyID]
	if !exists {
		return nil, ErrSecretDecrypt
	}
	wrapNonceSize := master.NonceSize()
	wrappedKeySize := dataKeySize + master.Overhead()
	dataNonceSize := 12 // AES-GCM standard nonce size used by Seal.
	minimum := 1 + wrapNonceSize + wrappedKeySize + dataNonceSize + 1 + 16
	if len(blob) < minimum || blob[0] != localEnvelopeVersion {
		return nil, ErrSecretDecrypt
	}
	offset := 1
	wrapNonce := blob[offset : offset+wrapNonceSize]
	offset += wrapNonceSize
	wrappedKey := blob[offset : offset+wrappedKeySize]
	offset += wrappedKeySize
	dataNonce := blob[offset : offset+dataNonceSize]
	offset += dataNonceSize
	sealedSecret := blob[offset:]
	authenticated := secretAAD(aad)
	dataKey, err := master.Open(nil, wrapNonce, wrappedKey, append([]byte("kave-v2-wrap\x00"), authenticated...))
	if err != nil || len(dataKey) != dataKeySize {
		if dataKey != nil {
			clear(dataKey)
		}
		return nil, ErrSecretDecrypt
	}
	defer clear(dataKey)
	dataBlock, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	dataAEAD, err := cipher.NewGCM(dataBlock)
	if err != nil || len(dataNonce) != dataAEAD.NonceSize() {
		return nil, ErrSecretDecrypt
	}
	plaintext, err := dataAEAD.Open(nil, dataNonce, sealedSecret, authenticated)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return plaintext, nil
}

func localMasterAEAD(encoded string) (cipher.AEAD, string, error) {
	key, err := decodeMasterKey(encoded)
	if err != nil {
		return nil, "", err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", fmt.Errorf("v2 postgres: create master cipher: %w", err)
	}
	master, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", fmt.Errorf("v2 postgres: create master AEAD: %w", err)
	}
	digest := sha256.Sum256(key)
	return master, "local-sha256:" + hex.EncodeToString(digest[:8]), nil
}

func decodeMasterKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("v2 postgres: explicit 32-byte master key is required")
	}
	decoders := []func(string) ([]byte, error){
		hex.DecodeString,
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range decoders {
		key, err := decode(encoded)
		if err == nil && len(key) == dataKeySize {
			return key, nil
		}
		if key != nil {
			clear(key)
		}
	}
	return nil, errors.New("v2 postgres: master key must encode exactly 32 bytes as hex or base64")
}

func secretAAD(aad SecretAAD) []byte {
	return []byte(fmt.Sprintf("kave-v2-secret\x00%s\x00%s\x00%s\x00%d", aad.AccountID, aad.NamespaceID, aad.Name, aad.Version))
}
