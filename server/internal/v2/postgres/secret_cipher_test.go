package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
)

func TestLocalEnvelopeCipherRoundTripAndBinding(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, err := NewLocalEnvelopeCipher(hex.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	aad := SecretAAD{AccountID: "account/acme", NamespaceID: "namespace/prod", Name: "openai", Version: 3}
	blob, keyID, err := cipher.Seal(context.Background(), aad, []byte("provider-secret"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Open(context.Background(), aad, blob, keyID)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "provider-secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}

	for name, mutate := range map[string]func(SecretAAD, []byte) (SecretAAD, []byte){
		"ciphertext": func(a SecretAAD, b []byte) (SecretAAD, []byte) { b[len(b)-1] ^= 1; return a, b },
		"namespace":  func(a SecretAAD, b []byte) (SecretAAD, []byte) { a.NamespaceID = "namespace/other"; return a, b },
		"version":    func(a SecretAAD, b []byte) (SecretAAD, []byte) { a.Version++; return a, b },
	} {
		t.Run(name, func(t *testing.T) {
			candidateAAD, candidateBlob := mutate(aad, append([]byte(nil), blob...))
			if _, err := cipher.Open(context.Background(), candidateAAD, candidateBlob, keyID); !errors.Is(err, ErrSecretDecrypt) {
				t.Fatalf("Open() error = %v", err)
			}
		})
	}
}

func TestLocalEnvelopeCipherRequiresExplicitStrongKey(t *testing.T) {
	for _, key := range []string{"", "short", hex.EncodeToString(make([]byte, 31))} {
		if _, err := NewLocalEnvelopeCipher(key); err == nil {
			t.Fatalf("NewLocalEnvelopeCipher(%q) succeeded", key)
		}
	}
}

func TestLocalEnvelopeCipherUsesKeyedDomainSeparatedIdempotencyMAC(t *testing.T) {
	firstKey := make([]byte, 32)
	secondKey := make([]byte, 32)
	for i := range firstKey {
		firstKey[i] = byte(i + 1)
		secondKey[i] = byte(i + 2)
	}
	first, err := NewLocalEnvelopeCipher(hex.EncodeToString(firstKey))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLocalEnvelopeCipher(hex.EncodeToString(secondKey))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"plaintext":"guessable-provider-key"}`)
	one, err := first.SecretIdempotencyMAC(payload)
	if err != nil {
		t.Fatal(err)
	}
	replay, _ := first.SecretIdempotencyMAC(payload)
	two, _ := second.SecretIdempotencyMAC(payload)
	if one != replay || one == two || len(one) != 64 {
		t.Fatalf("unexpected MAC behavior: one=%q replay=%q two=%q", one, replay, two)
	}
}

func TestLocalEnvelopeCipherKeyringDecryptsOldVersionsAndKeepsMACStable(t *testing.T) {
	oldKey := make([]byte, 32)
	newKey := make([]byte, 32)
	idempotencyKey := make([]byte, 32)
	for i := range oldKey {
		oldKey[i] = byte(i + 1)
		newKey[i] = byte(i + 2)
		idempotencyKey[i] = byte(i + 3)
	}
	oldEncoded := hex.EncodeToString(oldKey)
	newEncoded := hex.EncodeToString(newKey)
	idempotencyEncoded := hex.EncodeToString(idempotencyKey)
	oldCipher, err := NewLocalEnvelopeCipherKeyring(LocalEnvelopeKeyring{
		CurrentKey: oldEncoded, IdempotencyKey: idempotencyEncoded,
	})
	if err != nil {
		t.Fatal(err)
	}
	aad := SecretAAD{AccountID: "account/acme", NamespaceID: "namespace/prod", Name: "openai", Version: 1}
	oldBlob, oldKeyID, err := oldCipher.Seal(context.Background(), aad, []byte("old-provider-secret"))
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := NewLocalEnvelopeCipherKeyring(LocalEnvelopeKeyring{
		CurrentKey: newEncoded, DecryptionKeys: []string{oldEncoded}, IdempotencyKey: idempotencyEncoded,
	})
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := rotated.Open(context.Background(), aad, oldBlob, oldKeyID)
	if err != nil || string(plaintext) != "old-provider-secret" {
		t.Fatalf("open old version after rotation = %q, %v", plaintext, err)
	}
	clear(plaintext)
	newBlob, newKeyID, err := rotated.Seal(context.Background(), aad, []byte("new-provider-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if newKeyID == oldKeyID {
		t.Fatal("rotated cipher sealed with the old wrapping key")
	}
	if _, err := oldCipher.Open(context.Background(), aad, newBlob, newKeyID); !errors.Is(err, ErrSecretDecrypt) {
		t.Fatalf("old keyring opened new ciphertext: %v", err)
	}
	payload := []byte("same-idempotent-secret-write")
	oldMAC, _ := oldCipher.SecretIdempotencyMAC(payload)
	newMAC, _ := rotated.SecretIdempotencyMAC(payload)
	if oldMAC != newMAC {
		t.Fatalf("idempotency MAC changed across encryption rotation: %q != %q", oldMAC, newMAC)
	}
}

func TestLocalEnvelopeCipherKeyringRequiresStableIdempotencyKeyForRotation(t *testing.T) {
	oldKey := hex.EncodeToString(make([]byte, 32))
	newRaw := make([]byte, 32)
	newRaw[0] = 1
	_, err := NewLocalEnvelopeCipherKeyring(LocalEnvelopeKeyring{
		CurrentKey: hex.EncodeToString(newRaw), DecryptionKeys: []string{oldKey},
	})
	if err == nil {
		t.Fatal("rotating keyring without stable idempotency key succeeded")
	}
}
