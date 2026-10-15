package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/workspace"
)

func TestRelativeSpecsIgnoreQuotedPaths(t *testing.T) {
	text := "const repoRoot = join(here, '../../../../../../../');\n" +
		"import assistantDesktopLinkManifest from '../../../../../assistant_desktop_link_components.json';\n"
	got := relativeSpecs(text)
	if len(got) != 1 || got[0] != "../../../../../assistant_desktop_link_components.json" {
		t.Fatalf("specs=%q", got)
	}
}

func TestResolveImportSkipsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	rel, isDir, ok, errMsg := resolveImport(ws, "shared/web/frontend", "shared/web/frontend/src/a.ts", "../../../..")
	if ok || isDir || errMsg != "" || rel != "" {
		t.Fatalf("root walk rel=%q dir=%v ok=%v err=%q", rel, isDir, ok, errMsg)
	}
}

func TestLockfileOutsideImporter(t *testing.T) {
	got := lockfileOutside("importers:\n\n  ../../packages/missing: {}\n")
	if len(got) != 1 || got[0] != "../../packages/missing" {
		t.Fatalf("got=%q", got)
	}
}

func TestClipCommandOutputKeepsTail(t *testing.T) {
	head := "HEAD-ONLY\n" + strings.Repeat("WARN pnpm.overrides progress line\n", 400)
	tail := "ERR_PNPM_PACKAGE_MANAGER_UNSAFE_IMPORTER_PATH\nRefusing to install importer with unsafe path key \"../../packages/kernel-offline-policy\"\n"
	got := clipCommandOutput(head + tail)
	if !strings.Contains(got, "ERR_PNPM_PACKAGE_MANAGER_UNSAFE_IMPORTER_PATH") {
		t.Fatalf("error clipped off: %s", got)
	}
	if strings.Contains(got, "HEAD-ONLY") {
		t.Fatal("kept the head of the log")
	}
}

func TestTempBuildDirStaysOnStoreDevice(t *testing.T) {
	parent := t.TempDir()
	store := filepath.Join(parent, ".pnpm-store", "v10")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, copyImport, err := tempBuildDirAt("pnpm", store)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if copyImport {
		t.Fatal("expected hardlink-capable directory, got copy fallback")
	}
	if !sameDevice(dir, store) {
		t.Fatalf("temp %s is not on the store device", dir)
	}
	if !strings.Contains(dir, parent) {
		t.Fatalf("temp %s not under %s", dir, parent)
	}
}
