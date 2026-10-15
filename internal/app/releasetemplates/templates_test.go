package releasetemplates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/releasetemplates"
	"iacpack/internal/diagnostic"
)

func TestProposeAndWriteFresh(t *testing.T) {
	dir := t.TempDir()
	prop := releasetemplates.Propose(releasetemplates.Request{Workspace: dir})
	if !prop.OK {
		t.Fatalf("propose diags=%v", prop.Diagnostics)
	}
	if prop.Wrote {
		t.Fatal("propose must not write")
	}
	if len(prop.Files) != 4 {
		t.Fatalf("files=%d want 4", len(prop.Files))
	}
	for _, f := range prop.Files {
		if f.Action != "create" {
			t.Fatalf("%s action=%s", f.Path, f.Action)
		}
		abs := filepath.Join(dir, filepath.FromSlash(f.Path))
		if _, err := os.Lstat(abs); !os.IsNotExist(err) {
			t.Fatalf("propose wrote %s: %v", f.Path, err)
		}
	}

	wr := releasetemplates.Write(releasetemplates.Request{Workspace: dir})
	if !wr.OK || !wr.Wrote {
		t.Fatalf("write ok=%v wrote=%v diags=%v", wr.OK, wr.Wrote, wr.Diagnostics)
	}
	for _, rel := range releasetemplates.TemplateRelPaths() {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		raw, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) == 0 {
			t.Fatalf("empty %s", rel)
		}
		low := strings.ToLower(string(raw))
		for _, bad := range []string{"gh release create", "git push", "docker login", "curl | sh", "curl|sh"} {
			if strings.Contains(low, bad) {
				t.Fatalf("%s contains forbidden %q", rel, bad)
			}
		}
	}
	wf, _ := os.ReadFile(filepath.Join(dir, ".github/workflows/iacpack-ci.yml"))
	if !strings.Contains(string(wf), "actions/checkout@11d5960a326750d5838078e36cf38b85af677262") {
		t.Fatal("workflow missing pinned checkout")
	}
	if strings.Contains(string(wf), "@main") || strings.Contains(string(wf), "@master") {
		t.Fatal("floating action tag")
	}
}

func TestOverwriteRefusalAndForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".github/workflows/iacpack-ci.yml")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := releasetemplates.Write(releasetemplates.Request{Workspace: dir})
	if res.OK {
		t.Fatal("expected refusal without force")
	}
	assertCode(t, res.Diagnostics, diagnostic.CodeInputIOError)

	res2 := releasetemplates.Write(releasetemplates.Request{Workspace: dir, Force: true})
	if !res2.OK || !res2.Wrote {
		t.Fatalf("force write failed: %v", res2.Diagnostics)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) == "old\n" {
		t.Fatal("force did not replace")
	}
}

func TestGitignoreAppendMarkers(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := releasetemplates.Write(releasetemplates.Request{Workspace: dir})
	if !res.OK {
		t.Fatalf("diags=%v", res.Diagnostics)
	}
	raw, _ := os.ReadFile(gi)
	s := string(raw)
	if !strings.Contains(s, "node_modules/") || !strings.Contains(s, "# BEGIN iacpack") {
		t.Fatalf("append failed: %s", s)
	}
	// second write should skip
	res2 := releasetemplates.Write(releasetemplates.Request{Workspace: dir})
	if !res2.OK {
		t.Fatalf("second: %v", res2.Diagnostics)
	}
	for _, f := range res2.Files {
		if f.Path == ".gitignore" && f.Action != "skip" {
			t.Fatalf("gitignore action=%s want skip", f.Action)
		}
	}
}

func TestExamplesDrift(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "release-templates")
	pairs := map[string]string{
		"iacpack-ci.yml":        "data/iacpack-ci.yml",
		"iacpack-release.md":    "data/iacpack-release.md",
		"gitignore.snippet":     "data/gitignore.snippet",
		"gitattributes.snippet": "data/gitattributes.snippet",
	}
	for ex, emb := range pairs {
		want, err := os.ReadFile(filepath.Join(root, ex))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("data", filepath.Base(emb)))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Fatalf("drift %s vs embedded %s", ex, emb)
		}
	}
}

func assertCode(t *testing.T, diags []diagnostic.Diagnostic, code string) {
	t.Helper()
	for _, d := range diags {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing %s in %#v", code, diags)
}
