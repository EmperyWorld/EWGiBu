package secretfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateShapes(t *testing.T) {
	for range 20 {
		pw, err := Generate(KindPassword, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != passwordLen || strings.ContainsAny(pw, "IlO01:@/?#\n ") {
			t.Fatalf("password %q", pw)
		}
		for _, c := range pw {
			if !strings.ContainsRune(passwordAlphabet, c) {
				t.Fatalf("password charset %q", pw)
			}
		}
		name, err := Generate(KindName, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(name) != nameLen || name[0] < 'a' || name[0] > 'z' {
			t.Fatalf("name %q", name)
		}
		for _, c := range name {
			if !strings.ContainsRune(nameRest, c) {
				t.Fatalf("name charset %q", name)
			}
		}
		port, err := Generate(KindPort, nil)
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < portMin || n > portMax || strings.Contains(port, "\n") {
			t.Fatalf("port %q", port)
		}
	}
	if _, err := Generate("token", bytes.NewReader(make([]byte, 64))); !errors.Is(err, ErrBadKind) {
		t.Fatalf("kind err %v", err)
	}
}

func TestWriteFileAndRefuseLeakPaths(t *testing.T) {
	dir := t.TempDir()
	rel, err := Write(dir, "db-password", KindPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "secrets/db-password" {
		t.Fatalf("rel %s", rel)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "secrets", "db-password"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != passwordLen || bytes.Contains(raw, []byte("\n")) {
		t.Fatalf("file %q", raw)
	}
	info, err := os.Stat(filepath.Join(dir, "secrets", "db-password"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %o", info.Mode().Perm())
	}
	dinfo, err := os.Stat(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	if dinfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %o", dinfo.Mode().Perm())
	}
	if _, err := Write(dir, "db-password", KindPassword, false); !errors.Is(err, ErrExists) {
		t.Fatalf("exists %v", err)
	}
	rel, err = Write(dir, "db-password", KindName, true)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "secrets/db-password" {
		t.Fatal(rel)
	}
	next, err := os.ReadFile(filepath.Join(dir, "secrets", "db-password"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(next, raw) || len(next) != nameLen {
		t.Fatalf("replace %q", next)
	}
	if _, err := Write(dir, "../x", KindPassword, false); !errors.Is(err, ErrBadID) {
		t.Fatalf("traversal %v", err)
	}
	if _, err := Write(dir, "Bad_ID", KindPassword, false); !errors.Is(err, ErrBadID) {
		t.Fatalf("id %v", err)
	}
}

func TestWriteRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "secrets")); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "db-password", KindPassword, true); !errors.Is(err, ErrSymlink) {
		t.Fatalf("dir symlink %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote outside: %v", entries)
	}

	dir = t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "secrets"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "leak")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "secrets", "db-password")); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "db-password", KindPassword, true); !errors.Is(err, ErrSymlink) {
		t.Fatalf("file symlink %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("target changed %q", got)
	}
}
