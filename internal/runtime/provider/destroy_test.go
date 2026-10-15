package provider_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/runtime/process"
	"iacpack/internal/runtime/provider"
)

func TestHostDestroyDownArgv(t *testing.T) {
	t.Parallel()
	rec := &process.RecordingRunner{}
	h := provider.HostDestroy{Host: provider.Host{Runner: rec, Engine: "docker", Socket: "/tmp/test.sock"}}
	compose := filepath.Join(t.TempDir(), "compose.yaml")
	if err := h.Down(context.Background(), "iacp-demo-default-abc", compose); err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) != 1 {
		t.Fatalf("calls=%d", len(rec.Calls))
	}
	args := rec.Calls[0].Spec.Args
	want := []string{"compose", "-p", "iacp-demo-default-abc", "-f", compose, "down"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("argv=%v want %v", args, want)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--volumes") {
		t.Fatal("default down must not pass --volumes")
	}
}

func TestHostDestroyDeleteVolumesArgv(t *testing.T) {
	t.Parallel()
	rec := &process.RecordingRunner{}
	h := provider.HostDestroy{Host: provider.Host{Runner: rec, Engine: "docker", Socket: "/tmp/test.sock"}}
	if err := h.DeleteManagedVolumes(context.Background(), "proj", []string{"vol-a", "vol-b"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) != 2 {
		t.Fatalf("calls=%d", len(rec.Calls))
	}
	if got := strings.Join(rec.Calls[0].Spec.Args, " "); got != "volume rm vol-a" {
		t.Fatalf("first=%s", got)
	}
	if got := strings.Join(rec.Calls[1].Spec.Args, " "); got != "volume rm vol-b" {
		t.Fatalf("second=%s", got)
	}
}
