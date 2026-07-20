package v2_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"

	v2 "github.com/kave-io/kave/core/v2"
)

func TestServiceKeyMaterialIsCanonicalAndOneWay(t *testing.T) {
	t.Parallel()
	material, err := v2.GenerateServiceKeyMaterial(bytes.NewReader(bytes.Repeat([]byte{0x5a}, 50)))
	if err != nil {
		t.Fatal(err)
	}
	if len(material.LookupPrefix) != 24 || len(material.RawKey) != len("kv2_")+24+1+43 {
		t.Fatalf("generated material = prefix %q raw length %d", material.LookupPrefix, len(material.RawKey))
	}
	parsed, err := v2.ParseServiceKeyMaterial(material.RawKey)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.LookupPrefix != material.LookupPrefix || parsed.SecretHash != material.SecretHash {
		t.Fatalf("parsed material = %+v, want %+v", parsed, material)
	}
	digest := sha256.Sum256([]byte(material.RawKey))
	if material.SecretHash != digest {
		t.Fatal("verifier is not SHA-256(raw key)")
	}
}

func TestServiceKeyMaterialRejectsNonCanonicalOrWeakInput(t *testing.T) {
	t.Parallel()
	valid, err := v2.GenerateServiceKeyMaterial(bytes.NewReader(bytes.Repeat([]byte{0x42}, 50)))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"", "kv2_short.secret", valid.RawKey + "\n", valid.RawKey + "=", "other_" + valid.RawKey,
	} {
		if _, err := v2.ParseServiceKeyMaterial(raw); err == nil {
			t.Fatalf("ParseServiceKeyMaterial(%q) succeeded", raw)
		}
	}
	if err := v2.ValidateServiceKeyVerifier(valid.LookupPrefix, valid.SecretHash[:31]); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("short verifier error = %v", err)
	}
	zero := make([]byte, sha256.Size)
	if err := v2.ValidateServiceKeyVerifier(valid.LookupPrefix, zero); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("zero verifier error = %v", err)
	}
}
