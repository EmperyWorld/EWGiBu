package provider

import (
	"context"
	"testing"

	"iacpack/internal/runtime/process"
)

func TestResolveContainerEngineExplicit(t *testing.T) {
	e, err := ResolveContainerEngine("docker")
	if err != nil || e != EngineDocker {
		t.Fatalf("got %q err=%v", e, err)
	}
}

func TestSelectContainerSocketPodmanDefault(t *testing.T) {
	s, err := SelectContainerSocket(EnginePodman, "")
	if err != nil {
		t.Fatal(err)
	}
	if s == "/var/run/docker.sock" {
		t.Fatalf("expected podman socket path, got %q", s)
	}
}

func TestHostCLIUsesEngine(t *testing.T) {
	rec := &process.RecordingRunner{}
	h := Host{Runner: rec, Engine: string(EnginePodman), Socket: "/tmp/fake.sock"}
	_, err := h.DockerVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) == 0 || rec.Calls[0].Spec.Path != "podman" {
		t.Fatalf("calls=%v", rec.Calls)
	}
}
