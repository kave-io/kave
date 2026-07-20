package postgres

import (
	"strings"
	"testing"
)

func TestVerifyRuntimeMigrationSetRequiresExactBinaryManifest(t *testing.T) {
	t.Parallel()
	expected := []Migration{
		{Version: 1, Name: "001_kernel.up.sql", Checksum: strings.Repeat("a", 64)},
		{Version: 2, Name: "002_accounting.up.sql", Checksum: strings.Repeat("b", 64)},
	}
	if err := verifyRuntimeMigrationSet(expected, append([]Migration(nil), expected...)); err != nil {
		t.Fatalf("exact manifest rejected: %v", err)
	}
	for _, test := range []struct {
		name    string
		applied []Migration
	}{
		{name: "behind", applied: expected[:1]},
		{name: "ahead", applied: append(append([]Migration(nil), expected...), Migration{Version: 3, Name: "003_future.up.sql", Checksum: strings.Repeat("c", 64)})},
		{name: "checksum drift", applied: []Migration{expected[0], {Version: 2, Name: expected[1].Name, Checksum: strings.Repeat("0", 64)}}},
		{name: "name drift", applied: []Migration{expected[0], {Version: 2, Name: "002_other.up.sql", Checksum: expected[1].Checksum}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := verifyRuntimeMigrationSet(expected, test.applied); err == nil {
				t.Fatal("mismatched migration set accepted")
			}
		})
	}
}
