package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandSecretsDirUsesFileNameAndExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "db-password"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jwt-secret-key"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skip-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	pairs, diags := Expand(dir, []string{"db-password=/explicit/db"})
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	joined := strings.Join(pairs, "\n")
	if !strings.Contains(joined, "db-password=/explicit/db") {
		t.Fatalf("explicit override missing: %s", joined)
	}
	if strings.Contains(joined, filepath.Join(dir, "db-password")) {
		t.Fatalf("dir path should be replaced: %s", joined)
	}
	if !strings.Contains(joined, "jwt-secret-key=") {
		t.Fatalf("dir secret missing: %s", joined)
	}
	if strings.Contains(joined, "nope") {
		t.Fatal("secret bytes leaked into the expanded args")
	}
}
