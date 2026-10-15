package apply

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequireSecretFilesBeforeTask(t *testing.T) {
	root := t.TempDir()
	secrets := filepath.Join(root, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "postgres-password"), []byte("s3cret\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := RequireSecretFiles(secrets, []string{"postgres-password"}); err != nil {
		t.Fatal(err)
	}
	if err := RequireSecretFiles(secrets, []string{"missing"}); err == nil {
		t.Fatal("missing secret must fail before compose run")
	}
}
