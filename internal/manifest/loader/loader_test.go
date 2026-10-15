package loader_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

func TestPositiveYAMLAndJSONSameCanonical(t *testing.T) {
	root := repoRoot(t)
	yamlBytes, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	jsonBytes, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.json"))
	if err != nil {
		t.Fatal(err)
	}
	yd := loader.Parse("minimal.yaml", yamlBytes)
	jd := loader.Parse("minimal.json", jsonBytes)
	if len(yd.Diags) != 0 {
		t.Fatalf("yaml diags: %+v", yd.Diags)
	}
	if len(jd.Diags) != 0 {
		t.Fatalf("json diags: %+v", jd.Diags)
	}
	if !deepEqual(yd.Root.AsCanonical(), jd.Root.AsCanonical()) {
		t.Fatalf("canonical mismatch\nyaml=%#v\njson=%#v", yd.Root.AsCanonical(), jd.Root.AsCanonical())
	}
}

func TestNegativeFixtures(t *testing.T) {
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
		{"too_deep.yaml", diagnostic.CodeManifestTooDeep},
		{"too_large.yaml", diagnostic.CodeManifestTooLarge},
	}
	root := filepath.Join(repoRoot(t), "testdata/manifests/negative")
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			doc := loader.Parse(tc.file, data)
			if !hasCode(doc.Diags, tc.code) {
				t.Fatalf("want code %s, got %+v", tc.code, doc.Diags)
			}
		})
	}
}

func TestTooLarge(t *testing.T) {
	data := bytes.Repeat([]byte("a: 1\n"), (loader.MaxBytes/4)+1)
	doc := loader.Parse("big.yaml", data)
	if !hasCode(doc.Diags, diagnostic.CodeManifestTooLarge) {
		t.Fatalf("diags=%+v", doc.Diags)
	}
}

func TestTooDeep(t *testing.T) {
	var b strings.Builder
	depth := loader.MaxDepth + 5
	for i := 0; i < depth; i++ {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString("k:\n")
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("v: 1\n")
	doc := loader.Parse("deep.yaml", []byte(b.String()))
	if !hasCode(doc.Diags, diagnostic.CodeManifestTooDeep) {
		t.Fatalf("diags=%+v", doc.Diags)
	}
}

func TestMultipleDocuments(t *testing.T) {
	data := []byte("a: 1\n---\nb: 2\n")
	doc := loader.Parse("multi.yaml", data)
	if !hasCode(doc.Diags, diagnostic.CodeManifestMultipleDocuments) {
		t.Fatalf("diags=%+v", doc.Diags)
	}
}

func TestBlockSequenceMapValue(t *testing.T) {
	data := []byte(`spec:
  services:
    app:
      dependsOn:
        - target: db
          condition: healthy
      secrets:
        - ref: db-password
          target: /run/secrets/db-password
`)
	doc := loader.Parse("seq.yaml", data)
	if len(doc.Diags) != 0 {
		t.Fatalf("diags=%+v", doc.Diags)
	}
	deps := doc.Root.GetMap("spec").GetMap("services").GetMap("app").GetMap("dependsOn")
	if deps == nil || deps.Kind != loader.KindSeq || len(deps.Seq) != 1 {
		t.Fatalf("dependsOn=%#v", deps)
	}
}

func hasCode(diags []diagnostic.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
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
