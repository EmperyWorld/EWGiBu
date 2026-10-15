package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/diagnostic"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type testFS struct {
	files map[string][]byte
}

func (t *testFS) ReadFile(path string) ([]byte, error) {
	b, ok := t.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func (t *testFS) Exists(path string) (bool, error) {
	_, ok := t.files[path]
	return ok, nil
}

func (t *testFS) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if _, ok := t.files[path]; ok {
		return os.ErrExist
	}
	if t.files == nil {
		t.files = map[string][]byte{}
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	t.files[path] = cp
	_ = perm
	return nil
}

func (t *testFS) WriteFileAtomicReplace(path string, data []byte, perm os.FileMode) error {
	if t.files == nil {
		t.files = map[string][]byte{}
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	t.files[path] = cp
	_ = perm
	return nil
}

func TestInitAndValidateRoundTrip(t *testing.T) {
	fs := &testFS{files: map[string][]byte{}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"init", "--output", "iacpack.yaml", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("init exit=%d stderr=%s", code, stderr.String())
	}
	if _, ok := fs.files["iacpack.yaml"]; !ok {
		t.Fatal("file not written")
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "init.schema.json")

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{"validate", "--file", "iacpack.yaml", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("validate exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "validate.schema.json")
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["ok"] != true || env["command"] != "validate" {
		t.Fatalf("env=%v", env)
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	fs := &testFS{files: map[string][]byte{"iacpack.yaml": []byte("x")}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"init", "--output", "iacpack.yaml"}, stdout, stderr, fs)
	if code != cli.ExitIO {
		t.Fatalf("exit=%d", code)
	}
}

func TestValidateNegativeExitCode(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testdata/manifests/negative/mutable_tag.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fs := &testFS{files: map[string][]byte{"bad.yaml": data}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"validate", "--file", "bad.yaml", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitInvalid {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "validate.schema.json")
	if !strings.Contains(stdout.String(), diagnostic.CodeManifestMutableImageTag) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestSchemaExport(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"schema", "export", "--output", "json"}, stdout, stderr)
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "schema_export.schema.json")
	assertNoForeignURL(t, stdout.Bytes())
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["command"] != "schema export" {
		t.Fatalf("command=%v", env["command"])
	}
	result := env["result"].(map[string]any)
	if result["id"] != "iacpack.schemas.manifest.v1alpha1" {
		t.Fatalf("id=%v", result["id"])
	}
}

func TestValidateMissingFileUsage(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"validate"}, stdout, stderr)
	if code != cli.ExitUsage {
		t.Fatalf("exit=%d", code)
	}
}

func TestExitCodesWired(t *testing.T) {
	t.Run("usage=2", func(t *testing.T) {
		code := cli.ExecuteWith([]string{"validate"}, io.Discard, io.Discard)
		if code != cli.ExitUsage {
			t.Fatalf("exit=%d", code)
		}
	})
	t.Run("invalid=3", func(t *testing.T) {
		root := repoRoot(t)
		data, err := os.ReadFile(filepath.Join(root, "testdata/manifests/negative/unknown_field.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		fs := &testFS{files: map[string][]byte{"bad.yaml": data}}
		code := cli.ExecuteWithFS([]string{"validate", "--file", "bad.yaml"}, io.Discard, io.Discard, fs)
		if code != cli.ExitInvalid {
			t.Fatalf("exit=%d", code)
		}
	})
	t.Run("io=4", func(t *testing.T) {
		fs := &testFS{files: map[string][]byte{}}
		code := cli.ExecuteWithFS([]string{"validate", "--file", "missing.yaml"}, io.Discard, io.Discard, fs)
		if code != cli.ExitIO {
			t.Fatalf("exit=%d", code)
		}
	})
	t.Run("internal=70", func(t *testing.T) {
		code := cli.ExecuteWith([]string{"version", "--output", "json"}, errWriter{}, io.Discard)
		if code != cli.ExitInternal {
			t.Fatalf("exit=%d", code)
		}
	})
}

func TestNegativeFixturesStableCodes(t *testing.T) {
	cases := []struct {
		file string
		code string
	}{
		{"malformed.yaml", diagnostic.CodeManifestParseError},
		{"malformed.json", diagnostic.CodeManifestParseError},
		{"duplicate_key.yaml", diagnostic.CodeManifestDuplicateKey},
		{"alias.yaml", diagnostic.CodeManifestAliasForbidden},
		{"anchor.yaml", diagnostic.CodeManifestAnchorForbidden},
		{"merge_key.yaml", diagnostic.CodeManifestMergeKeyForbidden},
		{"unknown_field.yaml", diagnostic.CodeSchemaUnknownField},
		{"missing_required.yaml", diagnostic.CodeSchemaMissingField},
		{"bad_identifier.yaml", diagnostic.CodeSchemaInvalidIdentifier},
		{"bad_semver.yaml", diagnostic.CodeSchemaInvalidSemver},
		{"unsupported_version.yaml", diagnostic.CodeManifestUnsupportedAPI},
		{"unsupported_profile.yaml", diagnostic.CodeManifestUnsupportedProfile},
		{"mutable_tag.yaml", diagnostic.CodeManifestMutableImageTag},
		{"too_deep.yaml", diagnostic.CodeManifestTooDeep},
		{"too_large.yaml", diagnostic.CodeManifestTooLarge},
	}
	dir := filepath.Join(repoRoot(t), "testdata/manifests/negative")
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			fs := &testFS{files: map[string][]byte{tc.file: data}}
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			code := cli.ExecuteWithFS([]string{"validate", "--file", tc.file, "--output", "json"}, stdout, stderr, fs)
			if code != cli.ExitInvalid {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			validateAgainstCheckedInSchema(t, stdout.Bytes(), "validate.schema.json")
			if !strings.Contains(stdout.String(), tc.code) {
				t.Fatalf("want %s in %s", tc.code, stdout.String())
			}
		})
	}
}

func TestInitRealFSAtomicRefuseOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iacpack.yaml")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	fs := filesystem.OS{}
	code := cli.ExecuteWithFS([]string{"init", "--output", path}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("init exit=%d stderr=%s", code, stderr.String())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code = cli.ExecuteWithFS([]string{"init", "--output", path}, stdout, stderr, fs)
	if code != cli.ExitIO {
		t.Fatalf("overwrite exit=%d", code)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("file mutated after refused overwrite")
	}
}
