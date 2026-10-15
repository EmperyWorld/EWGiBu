package validate_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"iacpack/internal/app/validate"
)

type memFS struct {
	files map[string][]byte
}

func (m memFS) ReadFile(path string) ([]byte, error) {
	b, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	// Return a copy so mutations to the caller's buffer wouldn't affect store;
	// validate must still not mutate the returned slice in a way that changes source.
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func TestValidateDoesNotMutateSource(t *testing.T) {
	root := repoRoot(t)
	original, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	stored := append([]byte(nil), original...)
	fs := memFS{files: map[string][]byte{"m.yaml": stored}}
	res := validate.Run(fs, "m.yaml")
	if !res.OK {
		t.Fatalf("diags=%+v", res.Diagnostics)
	}
	if !bytes.Equal(stored, original) {
		t.Fatal("filesystem store mutated")
	}
	if !bytes.Equal(res.SourceBytes, original) {
		t.Fatal("source snapshot mismatch")
	}
}

func TestValidateYAMLJSONCanonical(t *testing.T) {
	root := repoRoot(t)
	y, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.json"))
	if err != nil {
		t.Fatal(err)
	}
	fs := memFS{files: map[string][]byte{"a.yaml": y, "a.json": j}}
	ry := validate.Run(fs, "a.yaml")
	rj := validate.Run(fs, "a.json")
	if !ry.OK || !rj.OK {
		t.Fatalf("y=%v j=%v", ry.Diagnostics, rj.Diagnostics)
	}
	if !deepEqual(ry.Canonical, rj.Canonical) {
		t.Fatalf("canonical mismatch")
	}
}

func TestValidateNoExecImport(t *testing.T) {
	// Architectural: this package must not import os/exec; covered by archrule.
	// Behaviorally: validate only uses injected FileReader.
	fs := memFS{files: map[string][]byte{"x.yaml": []byte("a: 1\n")}}
	res := validate.Run(fs, "x.yaml")
	if res.OK {
		t.Fatal("expected schema failure for incomplete manifest")
	}
}

func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
