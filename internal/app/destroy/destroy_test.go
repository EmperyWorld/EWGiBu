package destroy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/destroy"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/state"
)

func TestDestroyDryRunAndRetain(t *testing.T) {
	rec := &destroy.Recording{}
	res := destroy.Run(context.Background(), destroy.Options{
		Project: "demo", DryRun: true, DeleteVolumes: true, Yes: true,
		Provider: rec, RetentionDelete: []string{"cache"}, RetentionRetain: []string{"data"},
		StateHome: t.TempDir(),
	})
	if !res.OK {
		t.Fatalf("%v", res.Diagnostics)
	}
	if len(rec.Calls) != 0 {
		t.Fatal("dry-run must not call provider")
	}
	res = destroy.Run(context.Background(), destroy.Options{
		Project: "demo", DeleteVolumes: true, Yes: false, Provider: rec, StateHome: t.TempDir(),
	})
	if res.OK {
		t.Fatal("delete-volumes without yes must fail")
	}
}

func TestDestroyExec(t *testing.T) {
	rec := &destroy.Recording{}
	home := t.TempDir()
	paths := state.ForInstance(home, "demo", identity.DefaultInstance)
	if err := os.MkdirAll(paths.Rendered, 0o700); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(paths.Rendered, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := destroy.Run(context.Background(), destroy.Options{
		Project: "demo", Provider: rec, StateHome: home, Yes: true,
	})
	if !res.OK {
		t.Fatalf("%v", res.Diagnostics)
	}
	foundDown := false
	foundCompose := false
	for _, c := range rec.Calls {
		if strings.HasPrefix(c, "down:") {
			foundDown = true
		}
		if c == "compose:"+compose {
			foundCompose = true
		}
	}
	if !foundDown {
		t.Fatalf("expected down, got %v", rec.Calls)
	}
	if !foundCompose {
		t.Fatalf("expected compose path, got %v", rec.Calls)
	}
}
