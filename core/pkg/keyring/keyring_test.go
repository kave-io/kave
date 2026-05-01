package keyring

import (
	"context"
	"errors"
	"os"
	"testing"
)

// setHome overrides HOME for the test, routing plaintext fallbacks to a temp dir.
func setHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
}

func TestGetOrCreateMasterKey_disabled(t *testing.T) {
	t.Setenv("KAVE_KEYRING_DISABLED", "1")
	_, err := GetOrCreateMasterKey(context.Background())
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("expected ErrKeyringUnavailable, got %v", err)
	}
}

// TestGetOrCreateMasterKey_disabledIgnoresPlaintext verifies that KAVE_KEYRING_DISABLED=1
// causes an immediate ErrKeyringUnavailable even when KAVE_ALLOW_PLAINTEXT_KEY=1.
// The master key has no dev-mode fallback when the keyring is explicitly disabled.
func TestGetOrCreateMasterKey_disabledIgnoresPlaintext(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")
	t.Setenv("KAVE_ALLOW_PLAINTEXT_KEY", "1")

	_, err := GetOrCreateMasterKey(context.Background())
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("expected ErrKeyringUnavailable even with KAVE_ALLOW_PLAINTEXT_KEY=1, got %v", err)
	}
}

func TestSetGetDelete(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")

	svc, acc, val := "test-svc", "test-acc", "super-secret"

	if err := Set(svc, acc, val); err != nil {
		t.Fatal(err)
	}

	got, err := Get(svc, acc)
	if err != nil {
		t.Fatal(err)
	}
	if got != val {
		t.Fatalf("expected %q, got %q", val, got)
	}

	if err := Delete(svc, acc); err != nil {
		t.Fatal(err)
	}

	// after delete, reads should fail
	if _, err := Get(svc, acc); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestGet_missing(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")

	_, err := Get("nonexistent-svc", "no-account")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDelete_nonexistent(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")

	// deleting non-existent key should not error (idempotent)
	if err := Delete("ghost-svc", "ghost-acc"); err != nil {
		t.Fatalf("delete of non-existent key should succeed, got %v", err)
	}
}

func TestSet_overwrite(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")

	if err := Set("svc", "acc", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := Set("svc", "acc", "v2"); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("svc", "acc")
	if got != "v2" {
		t.Fatalf("expected v2 after overwrite, got %q", got)
	}
}

func TestPlaintextFallback_emptyHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("KAVE_KEYRING_DISABLED", "1")
	t.Setenv("KAVE_ALLOW_PLAINTEXT_KEY", "1")

	_, err := GetOrCreateMasterKey(context.Background())
	if err == nil {
		t.Fatal("expected error with empty HOME")
	}
}

func TestSet_differentAccounts(t *testing.T) {
	setHome(t)
	t.Setenv("KAVE_KEYRING_DISABLED", "1")

	if err := Set("svc", "acc1", "val1"); err != nil {
		t.Fatal(err)
	}
	if err := Set("svc", "acc2", "val2"); err != nil {
		t.Fatal(err)
	}

	v1, _ := Get("svc", "acc1")
	v2, _ := Get("svc", "acc2")

	if v1 != "val1" || v2 != "val2" {
		t.Fatalf("account isolation failed: v1=%q v2=%q", v1, v2)
	}
}

func init() {
	// Ensure OS keyring is never touched during tests by default.
	// Individual tests can override if needed.
	os.Setenv("KAVE_KEYRING_DISABLED", "1")
}
