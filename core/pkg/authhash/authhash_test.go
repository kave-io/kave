package authhash

import (
	"strings"
	"testing"
)

func TestHashPassword_roundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct-horse-battery-staple") {
		t.Fatal("verify should succeed with same password")
	}
}

func TestVerifyPassword_wrongPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("verify should fail with different password")
	}
}

func TestVerifyPassword_emptyHash(t *testing.T) {
	if VerifyPassword(nil, "anything") {
		t.Fatal("nil hash should return false")
	}
	if VerifyPassword([]byte{}, "anything") {
		t.Fatal("empty hash should return false")
	}
}

func TestVerifyPassword_truncatedHash(t *testing.T) {
	hash, _ := HashPassword("pw")
	if VerifyPassword(hash[:10], "pw") {
		t.Fatal("truncated hash should return false")
	}
}

func TestHashPassword_uniqueSalts(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if string(h1) == string(h2) {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestHashToken_deterministic(t *testing.T) {
	token := "kave_abc123xyz"
	h1 := HashToken(token)
	h2 := HashToken(token)
	if string(h1) != string(h2) {
		t.Fatal("HashToken must be deterministic")
	}
	if len(h1) == 0 {
		t.Fatal("hash must not be empty")
	}
}

func TestHashToken_differentInputs(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if string(h1) == string(h2) {
		t.Fatal("different tokens must produce different hashes")
	}
}

func TestGenerateToken_prefix(t *testing.T) {
	plain, hash, err := GenerateToken("pat_")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "pat_") {
		t.Fatalf("expected pat_ prefix, got %q", plain)
	}
	if len(hash) == 0 {
		t.Fatal("hash must not be empty")
	}
	// hash should match plain
	if string(HashToken(plain)) != string(hash) {
		t.Fatal("hash mismatch with HashToken(plain)")
	}
}

func TestGenerateToken_unique(t *testing.T) {
	p1, _, _ := GenerateToken("tok_")
	p2, _, _ := GenerateToken("tok_")
	if p1 == p2 {
		t.Fatal("two generated tokens must differ")
	}
}

func TestSubtleCompare(t *testing.T) {
	a := []byte("hello")
	b := []byte("hello")
	c := []byte("world")
	if !subtleCompare(a, b) {
		t.Fatal("equal slices should match")
	}
	if subtleCompare(a, c) {
		t.Fatal("different slices should not match")
	}
	if subtleCompare(a, []byte("hi")) {
		t.Fatal("different length should not match")
	}
}
