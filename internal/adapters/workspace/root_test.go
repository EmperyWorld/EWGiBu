package workspace

import (
	"path/filepath"
	"testing"
)

func TestRootForManifest(t *testing.T) {
	dir := t.TempDir()
	draft := filepath.Join(dir, "imports", "import-draft.json")
	got, err := RootForManifest(draft)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("draft root=%s want %s", got, dir)
	}
	got, err = RootForManifest(filepath.Join(dir, "iacpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("manifest root=%s want %s", got, dir)
	}
}
