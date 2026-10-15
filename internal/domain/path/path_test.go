package path_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"iacpack/internal/domain/path"
)

func TestParseWorkspacePathTable(t *testing.T) {
	ok := []string{"a", "a/b", "pkg/file.yaml", ".hidden", "x-y_z"}
	for _, s := range ok {
		p, err := path.ParseWorkspacePath(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if p.String() != s {
			t.Fatalf("string %q", p)
		}
	}
	bad := []string{
		"", "../x", "/abs", "a//b", "a/../b", `a\b`, "C:foo", "//unc",
		string([]byte{0xff, 0xfe}), "a\x00b",
	}
	for _, s := range bad {
		if _, err := path.ParseWorkspacePath(s); err == nil {
			t.Fatalf("expected reject %q", s)
		}
	}
}

func TestParseBundlePath(t *testing.T) {
	p, err := path.ParseBundlePath("artifacts/compose.yaml")
	if err != nil || p.String() != "artifacts/compose.yaml" {
		t.Fatalf("%v %v", p, err)
	}
	if _, err := path.ParseBundlePath("../escape"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestParseContainerPathEdges(t *testing.T) {
	if _, err := path.ParseContainerPath("/ok/path"); err != nil {
		t.Fatal(err)
	}
	bad := []string{"", "rel", "/a/../b", "/a//b", "/\\x", "a\x00"}
	for _, s := range bad {
		if _, err := path.ParseContainerPath(s); err == nil {
			t.Fatalf("expected reject %q", s)
		}
	}
	root, err := path.ParseContainerPath("/")
	if err != nil || root.String() != "/" {
		t.Fatalf("root container path: %v %v", root, err)
	}
}

func TestParseManagedAndExternal(t *testing.T) {
	m, err := path.ParseManagedHostPath("/var/lib/iacpack/x")
	if err != nil || m.String() != "/var/lib/iacpack/x" {
		t.Fatal(err)
	}
	if _, err := path.ParseManagedHostPath("relative"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := path.ParseManagedHostPath("/a\x00"); err == nil {
		t.Fatal("expected reject")
	}
	e, err := path.ParseExternalHostPath("/opt/data")
	if err != nil || e.String() != "/opt/data" {
		t.Fatal(err)
	}
	if _, err := path.ParseExternalHostPath("x\x00"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := path.ParseExternalHostPath(""); err == nil {
		t.Fatal("expected reject")
	}
}

func FuzzParseWorkspacePath(f *testing.F) {
	f.Add("ok/path")
	f.Add("../x")
	f.Add("/abs")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, raw string) {
		_, err := path.ParseWorkspacePath(raw)
		if err != nil {
			return
		}
		if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") ||
			strings.Contains(raw, "//") || strings.ContainsRune(raw, 0) || !utf8.ValidString(raw) {
			t.Fatalf("accepted unsafe %q", raw)
		}
		for _, seg := range strings.Split(raw, "/") {
			if seg == "" || seg == ".." {
				t.Fatalf("accepted unsafe segment in %q", raw)
			}
		}
		if len(raw) >= 2 && raw[1] == ':' {
			t.Fatalf("accepted drive prefix %q", raw)
		}
	})
}
