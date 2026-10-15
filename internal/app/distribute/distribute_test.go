package distribute_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/distribute"
)

func TestBuildDistribution(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "b.iacbundle")
	bin := filepath.Join(dir, "iacpack")
	if err := os.WriteFile(bundle, []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "dist")
	res := distribute.Build(distribute.Options{BundlePath: bundle, RuntimeBin: bin, OutputDir: out, Arch: "amd64"})
	if !res.OK {
		t.Fatal(res.Error)
	}
	script, err := os.ReadFile(filepath.Join(out, "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(script)
	for _, bad := range []string{"sudo", "curl", "eval", "source "} {
		if strings.Contains(s, bad) {
			t.Fatalf("install.sh contains %q", bad)
		}
	}
	if !strings.Contains(s, "sha256sum -c") || !strings.Contains(s, "exec") {
		t.Fatal("missing verify/exec")
	}
}
