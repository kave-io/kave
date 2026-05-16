package cert

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// updateGoldenEnv, when set to "1" or "true", causes golden assertions to
// overwrite the fixture on disk instead of comparing.
const updateGoldenEnv = "KAVE_UPDATE_GOLDEN"

func goldenUpdating() bool {
	v := os.Getenv(updateGoldenEnv)
	return v == "1" || v == "true"
}

// assertGoldenJSON marshals v with stable indentation and either writes the
// golden file (when KAVE_UPDATE_GOLDEN=1) or compares byte-for-byte.
func assertGoldenJSON(t *testing.T, path string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("golden marshal: %v", err)
	}
	got = append(got, '\n')

	if goldenUpdating() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("golden write: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden read %s: %v (re-run with %s=1 to create)", path, err, updateGoldenEnv)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s\n(re-run with %s=1 to refresh)",
			path, want, got, updateGoldenEnv)
	}
}
