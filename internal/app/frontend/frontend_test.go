package frontend_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/frontend"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/path"
)

type recRunner struct {
	calls       [][]string
	write       bool
	output      string
	sibling     string
	siblingMiss bool
}

func (r *recRunner) Run(dir string, argv []string) error {
	r.calls = append(r.calls, append([]string(nil), argv...))
	if r.sibling != "" {
		if _, err := os.Stat(filepath.Join(dir, r.sibling)); err != nil {
			r.siblingMiss = true
			return err
		}
	}
	if !r.write || len(argv) < 2 || argv[1] != "run" {
		return nil
	}
	name := r.output
	if name == "" {
		name = "dist"
	}
	out := filepath.Join(dir, name)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "index.html"), []byte("<html>ok</html>"), 0o644)
}

func sampleBuild(root string) (domain.PackageBuild, error) {
	if err := os.MkdirAll(filepath.Join(root, "shared/web/frontend"), 0o755); err != nil {
		return domain.PackageBuild{}, err
	}
	base := filepath.Join(root, "shared/web/frontend")
	if err := os.WriteFile(filepath.Join(base, "package.json"), []byte(`{"scripts":{"build":"echo ok"}}`), 0o644); err != nil {
		return domain.PackageBuild{}, err
	}
	if err := os.WriteFile(filepath.Join(base, "pnpm-lock.yaml"), []byte("lockfileVersion: 9\n"), 0o644); err != nil {
		return domain.PackageBuild{}, err
	}
	if err := os.WriteFile(filepath.Join(base, "main.js"), []byte("export {}\n"), 0o644); err != nil {
		return domain.PackageBuild{}, err
	}
	ctx, err := path.ParseWorkspacePath("shared/web/frontend")
	if err != nil {
		return domain.PackageBuild{}, err
	}
	return domain.PackageBuild{
		Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist", Entry: "index.html",
	}, nil
}

func TestMaterializeRunsManagerAndCachesNonEmptyOutput(t *testing.T) {
	root := t.TempDir()
	b, err := sampleBuild(root)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recRunner{write: true}
	cache := filepath.Join(t.TempDir(), "cache")
	builds := map[string]domain.PackageBuild{"spa": b}
	files, diags := frontend.Materialize(ws, builds, runner, cache)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if len(files["spa"]) != 1 || files["spa"][0].Rel != "index.html" || string(files["spa"][0].Body) == "" {
		t.Fatalf("files=%#v", files["spa"])
	}
	if len(runner.calls) != 2 || runner.calls[0][0] != "pnpm" || runner.calls[0][2] != "--frozen-lockfile" || runner.calls[1][2] != "build" {
		t.Fatalf("argv=%v", runner.calls)
	}
	before := len(runner.calls)
	files, diags = frontend.Materialize(ws, builds, runner, cache)
	if len(diags) != 0 || len(runner.calls) != before {
		t.Fatalf("cache miss diags=%v calls=%d", diags, len(runner.calls))
	}
	if string(files["spa"][0].Body) != "<html>ok</html>" {
		t.Fatalf("cached body=%q", files["spa"][0].Body)
	}
}

func TestMaterializeRejectsEmptyOutput(t *testing.T) {
	root := t.TempDir()
	b, err := sampleBuild(root)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recRunner{write: false}
	_, diags := frontend.Materialize(ws, map[string]domain.PackageBuild{"spa": b}, runner, "")
	if len(diags) != 1 || diags[0].Message == "" {
		t.Fatal(diags)
	}
}

func TestCheckRequiresPackageJSONAndLockfile(t *testing.T) {
	root := t.TempDir()
	ctx, _ := path.ParseWorkspacePath("shared/web/frontend")
	b := domain.PackageBuild{Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist", Entry: "index.html"}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	diags := frontend.Check(ws, map[string]domain.PackageBuild{"spa": b})
	if len(diags) == 0 {
		t.Fatal("expected missing package.json")
	}
}

func TestMaterializeCopiesWorkspacePackagesOutsideContext(t *testing.T) {
	root := t.TempDir()
	front := filepath.Join(root, "shared/web/frontend")
	pkg := filepath.Join(root, "shared/packages/kernel-offline-policy")
	if err := os.MkdirAll(front, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "package.json"), []byte(`{"scripts":{"build":"echo ok"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\n\nimporters:\n\n  .: {}\n\n  ../../packages/kernel-offline-policy: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-workspace.yaml"), []byte("packages:\n  - \".\"\n  - \"../../packages/*\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@eliteew/kernel-offline-policy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := path.ParseWorkspacePath("shared/web/frontend")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	b := domain.PackageBuild{Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist-build", Entry: "index.html"}
	runner := &recRunner{write: true, output: "dist-build", sibling: filepath.Join("..", "..", "packages", "kernel-offline-policy", "package.json")}
	files, diags := frontend.Materialize(ws, map[string]domain.PackageBuild{"spa": b}, runner, "")
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if runner.siblingMiss {
		t.Fatal("package outside context was not copied")
	}
	if len(files["spa"]) != 1 || files["spa"][0].Rel != "index.html" || len(files["spa"][0].Body) == 0 {
		t.Fatalf("files=%#v", files["spa"])
	}
}

func TestCheckRejectsMissingAndEscapingImporter(t *testing.T) {
	root := t.TempDir()
	front := filepath.Join(root, "shared/web/frontend")
	if err := os.MkdirAll(front, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "package.json"), []byte(`{"scripts":{"build":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-lock.yaml"), []byte("importers:\n\n  ../../packages/missing: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, _ := path.ParseWorkspacePath("shared/web/frontend")
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	b := domain.PackageBuild{Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist", Entry: "index.html"}
	diags := frontend.Check(ws, map[string]domain.PackageBuild{"spa": b})
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "../../packages/missing") {
		t.Fatalf("diags=%v", diags)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-lock.yaml"), []byte("importers:\n\n  ../../../../etc: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diags = frontend.Check(ws, map[string]domain.PackageBuild{"spa": b})
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "escapes the workspace") {
		t.Fatalf("escape diags=%v", diags)
	}
}

func TestMaterializeCopiesRelativeImportOutsideContext(t *testing.T) {
	root := t.TempDir()
	front := filepath.Join(root, "shared/web/frontend")
	admin := filepath.Join(front, "src/pages/Admin")
	desk := filepath.Join(front, "src/pages/Desktop/utils")
	if err := os.MkdirAll(admin, 0o755); err != nil || os.MkdirAll(desk, 0o755) != nil {
		t.Fatal(err)
	}
	jsonBody := []byte("{\"apps\":[\"desk\"]}\n")
	if err := os.WriteFile(filepath.Join(root, "shared/assistant_desktop_link_components.json"), jsonBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "package.json"), []byte(`{"scripts":{"build":"echo ok"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\nimporters:\n  .: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adminSrc := "import assistantDesktopLinkManifest from '../../../../../assistant_desktop_link_components.json';\nexport const x = assistantDesktopLinkManifest;\n"
	deskSrc := "import assistantDesktopLinkManifest from '../../../../../../assistant_desktop_link_components.json';\nexport const y = assistantDesktopLinkManifest;\n"
	if err := os.WriteFile(filepath.Join(admin, "systemGafInstructionShared.ts"), []byte(adminSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desk, "assistantDesktopLinkComponents.ts"), []byte(deskSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := path.ParseWorkspacePath("shared/web/frontend")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	b := domain.PackageBuild{Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist-build", Entry: "index.html"}
	runner := &recRunner{
		write:   true,
		output:  "dist-build",
		sibling: filepath.Join("..", "..", "assistant_desktop_link_components.json"),
	}
	files, diags := frontend.Materialize(ws, map[string]domain.PackageBuild{"spa": b}, runner, "")
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if runner.siblingMiss {
		t.Fatal("shared JSON outside context was not copied")
	}
	if len(files["spa"]) != 1 || string(files["spa"][0].Body) == "" {
		t.Fatalf("files=%#v", files["spa"])
	}
}

func TestMaterializeSkipsSymlinkInsteadOfFailing(t *testing.T) {
	root := t.TempDir()
	front := filepath.Join(root, "shared/web/frontend")
	if err := os.MkdirAll(front, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "package.json"), []byte(`{"scripts":{"build":"echo ok"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\nimporters:\n  .: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(front, "real.ts"), []byte("export const n = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.ts", filepath.Join(front, "alias.ts")); err != nil {
		t.Fatal(err)
	}
	ctx, err := path.ParseWorkspacePath("shared/web/frontend")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	b := domain.PackageBuild{Context: ctx, PackageManager: "pnpm", Install: "frozen", Script: "build", Output: "dist-build", Entry: "index.html"}
	runner := &recRunner{write: true, output: "dist-build"}
	files, diags := frontend.Materialize(ws, map[string]domain.PackageBuild{"spa": b}, runner, "")
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if len(files["spa"]) != 1 || len(files["spa"][0].Body) == 0 {
		t.Fatalf("files=%#v", files["spa"])
	}
}
