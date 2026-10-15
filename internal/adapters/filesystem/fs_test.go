package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/filesystem"
)

func TestWriteFileAtomicCreatesAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iacpack.yaml")
	fs := filesystem.OS{}
	data := []byte("apiVersion: iacpack.io/v1alpha1\n")

	if err := fs.WriteFileAtomic(path, data, 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("content mismatch: %q", got)
	}

	if err := fs.WriteFileAtomic(path, []byte("other\n"), 0o644); err == nil {
		t.Fatal("expected refuse overwrite")
	}
	got2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != string(data) {
		t.Fatalf("original overwritten: %q", got2)
	}
}

func TestWriteFileAtomicExclusiveAgainstPreexisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.yaml")
	if err := os.WriteFile(path, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	if err := fs.WriteFileAtomic(path, []byte("new\n"), 0o644); err == nil {
		t.Fatal("expected error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("mutated: %q", got)
	}
}
