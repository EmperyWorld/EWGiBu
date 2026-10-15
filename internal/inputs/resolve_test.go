package inputs_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/workspace"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/path"
	"iacpack/internal/inputs"
)

func TestResolveDeterministicAndIgnoresUndeclared(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "config", "app.yaml"), "a: 1\n")
	mustWrite(t, filepath.Join(dir, "config", "secret.env"), "X=1\n") // undeclared exclude
	mustWrite(t, filepath.Join(dir, "config", "other.yaml"), "b: 2\n")
	// create files in random-ish order already done; resolution sorts

	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {
				Source:  path.WorkspacePath("config"),
				Include: []string{"*.yaml"},
				Exclude: []string{"other.yaml"},
				Mode:    "0644",
			},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 1 || recs[0].Path != "config/app.yaml" {
		t.Fatalf("recs=%+v", recs)
	}
	sum := sha256.Sum256([]byte("a: 1\n"))
	if recs[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash=%s", recs[0].SHA256)
	}
}

func TestSymlinkInsideWorkspaceAccepted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "config", "app.yaml"), "x\n")
	if err := os.Symlink("app.yaml", filepath.Join(dir, "config", "link.yaml")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {Source: path.WorkspacePath("config"), Include: []string{"**"}},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 2 {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret"), "nope\n")
	mustMkdir(t, filepath.Join(dir, "config"))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "config", "leak")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {Source: path.WorkspacePath("config"), Include: []string{"**"}},
		},
	}}
	_, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	found := false
	for _, d := range diags {
		if d.Code == inputs.CodeInputForbiddenType {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected escaping symlink refusal, got %v", diags)
	}
}

func TestRejectCombiningCharacterPath(t *testing.T) {
	// e + combining acute
	err := inputs.ValidateRelPath("caf\u0301e.yaml")
	if err == nil {
		t.Fatal("expected combining mark rejection")
	}
}

func TestEmptyMatchFailsUnlessAllowed(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "config"))
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {Source: path.WorkspacePath("config"), Include: []string{"*.yaml"}},
		},
	}}
	_, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) == 0 {
		t.Fatal("expected empty match error")
	}
	proj.Spec.Resources["cfg"] = domain.Resource{
		Source: path.WorkspacePath("config"), Include: []string{"*.yaml"}, AllowEmpty: true,
	}
	_, diags = inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) != 0 {
		t.Fatalf("allowEmpty should pass: %v", diags)
	}
}

func TestCaseFoldCollision(t *testing.T) {
	// Only meaningful when FS can hold both names; on typical Linux we can.
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "config", "App.yaml"), "1\n")
	mustWrite(t, filepath.Join(dir, "config", "app.yaml"), "2\n")
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {Source: path.WorkspacePath("config"), Include: []string{"**"}},
		},
	}}
	_, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	found := false
	for _, d := range diags {
		if d.Code == inputs.CodeInputCollision && strings.Contains(d.Message, "case-folding") {
			found = true
		}
	}
	if !found {
		// Some filesystems may be case-insensitive and refuse the second write.
		if _, err := os.Stat(filepath.Join(dir, "config", "App.yaml")); err == nil {
			if _, err2 := os.Stat(filepath.Join(dir, "config", "app.yaml")); err2 == nil {
				t.Fatalf("expected collision, got %v", diags)
			}
		}
	}
}

func TestResolveKeepsSourceExecuteBit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker", "scripts", "log-to-file.sh")
	plain := filepath.Join(dir, "docker", "scripts", "note.txt")
	mustWrite(t, script, "#!/bin/sh\n")
	mustWrite(t, plain, "note\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"scripts": {Source: path.WorkspacePath("docker/scripts"), Include: []string{"**"}},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	modes := map[string]string{}
	for _, r := range recs {
		modes[r.Path] = r.Mode
	}
	if modes["docker/scripts/log-to-file.sh"] != "0755" {
		t.Fatalf("script mode=%q", modes["docker/scripts/log-to-file.sh"])
	}
	if modes["docker/scripts/note.txt"] != "0644" {
		t.Fatalf("plain mode=%q", modes["docker/scripts/note.txt"])
	}
}

func TestLiteralIncludeDoesNotScanUnmatchedTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "shared", "web", "frontend", "nginx.prod.conf"), "server {}\n")
	mods := filepath.Join(dir, "shared", "web", "frontend", "node_modules", "pkg")
	mustWrite(t, filepath.Join(mods, "LICENSE"), "mit\n")
	if err := os.Link(filepath.Join(mods, "LICENSE"), filepath.Join(mods, "LICENSE.hard")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret"), "nope\n")
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(mods, "leak")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"web": {
				Source:  path.WorkspacePath("shared/web/frontend"),
				Include: []string{"nginx.prod.conf"},
			},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 1 || recs[0].Path != "shared/web/frontend/nginx.prod.conf" {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestHardlinkIncludedAsFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "config", "app.yaml"), "x\n")
	if err := os.Link(filepath.Join(dir, "config", "app.yaml"), filepath.Join(dir, "config", "app-link.yaml")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"cfg": {Source: path.WorkspacePath("config"), Include: []string{"**"}},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 2 {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestSamePathInTwoResourcesKeepsBothSources(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "shared", "api", "main.go"), "package api\n")
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := domain.Resource{Source: path.WorkspacePath("shared/api"), Include: []string{"**"}}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"api":    res,
			"worker": res,
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 2 {
		t.Fatalf("recs=%+v", recs)
	}
	sources := map[string]struct{}{}
	for _, r := range recs {
		if r.Path != "shared/api/main.go" {
			t.Fatalf("path=%s", r.Path)
		}
		sources[r.Source] = struct{}{}
	}
	if _, ok := sources["resource/api"]; !ok {
		t.Fatal("missing api source")
	}
	if _, ok := sources["resource/worker"]; !ok {
		t.Fatal("missing worker source")
	}
}

func TestFileSourceIsNotADirectoryWalk(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "docker", "scripts", "internal-sites", "coredns", "Corefile"), ".:53\n")
	mustWrite(t, filepath.Join(dir, "docker", "scripts", "internal-sites", "coredns", "extra"), "no\n")
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj := domain.Project{Spec: domain.Spec{
		Resources: map[string]domain.Resource{
			"dns": {
				Source:  path.WorkspacePath("docker/scripts/internal-sites/coredns/Corefile"),
				Include: []string{"**"},
			},
		},
	}}
	recs, diags := inputs.ResolveProject(ws, proj, inputs.ResolveOptions{})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(recs) != 1 || recs[0].Path != "docker/scripts/internal-sites/coredns/Corefile" {
		t.Fatalf("recs=%+v", recs)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
