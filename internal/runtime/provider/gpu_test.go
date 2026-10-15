package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/runtime/process"
)

func TestPodmanCountAllCDIMissing(t *testing.T) {
	orig := cdiSpecDirs
	t.Cleanup(func() { cdiSpecDirs = orig })
	dir := t.TempDir()
	cdiSpecDirs = func() []string { return []string{dir} }

	h := HostApply{Host: Host{Engine: "podman"}}
	if !h.PodmanCountAllCDIMissing() {
		t.Fatal("empty spec dir must not satisfy nvidia.com/gpu=all")
	}
	body := []byte("{\"kind\":\"nvidia.com/gpu\",\"devices\":[{\"name\":\"all\"}]}\n")
	if err := os.WriteFile(filepath.Join(dir, "nvidia.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if h.PodmanCountAllCDIMissing() {
		t.Fatal("declared device all must be available")
	}
	docker := HostApply{Host: Host{Engine: "docker"}}
	if docker.PodmanCountAllCDIMissing() {
		t.Fatal("docker keeps count all")
	}
}

func TestComposeUpNamesServiceAndCDIDevice(t *testing.T) {
	rec := &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
		return process.Result{
			ExitCode: 1,
			Stderr: []byte("podman-compose version 1.6.0\n" +
				"Error: setting up CDI devices: unresolvable CDI devices nvidia.com/gpu=all\n"),
		}, nil
	}}
	h := HostApply{Host: Host{Runner: rec, Engine: "podman", Socket: "/tmp/test.sock"}}
	err := h.ComposeUp(context.Background(), "proj", "/tmp/compose.yaml", "llama-chat", nil)
	var cdi *CDIUnavailableError
	if !errors.As(err, &cdi) {
		t.Fatalf("got %v", err)
	}
	if cdi.Service != "llama-chat" || cdi.Device != NVIDIACountAllCDIDevice {
		t.Fatalf("service=%q device=%q", cdi.Service, cdi.Device)
	}
	if len(rec.Calls) != 1 {
		t.Fatalf("calls=%d", len(rec.Calls))
	}
	args := rec.Calls[0].Spec.Args
	want := "compose -p proj -f /tmp/compose.yaml up -d --no-deps --force-recreate llama-chat"
	if strings.Join(args, " ") != want {
		t.Fatalf("argv=%q", strings.Join(args, " "))
	}
}

func TestComposeUpRecreateArgv(t *testing.T) {
	rec := &process.RecordingRunner{}
	h := HostApply{Host: Host{Runner: rec, Engine: "podman", Socket: "/tmp/test.sock"}}
	if err := h.ComposeUpRecreate(context.Background(), "proj", "/tmp/compose.yaml", "llama-chat", nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) != 1 {
		t.Fatalf("calls=%d", len(rec.Calls))
	}
	got := strings.Join(rec.Calls[0].Spec.Args, " ")
	want := "compose -p proj -f /tmp/compose.yaml up -d --no-deps --force-recreate llama-chat"
	if got != want {
		t.Fatalf("argv=%q", got)
	}
}
