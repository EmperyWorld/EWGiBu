package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanKitWithoutBundlePassesCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "images.txt"), []byte("nginx\tpull\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), []byte(`[{"service":"app","target":"/models","parameter":"models-path","value":"/srv/models"}]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Run(Options{Bundle: dir})
	if !res.OK || !strings.Contains(res.Text, "portable check passed") {
		t.Fatalf("ok=%v text=%q diags=%v", res.OK, res.Text, res.Diagnostics)
	}
}

func TestPlanKitStopsOnEmptyHostPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "images.txt"), []byte("nginx\tpull\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), []byte(`[{"service":"app","target":"/models","parameter":"models-path","value":""}]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Run(Options{Bundle: dir})
	if res.OK {
		t.Fatal("empty host path should stop plan")
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "PORTABLE.HOST_PATH" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diags=%v", res.Diagnostics)
	}
}
