package apply_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/app/apply"
	"iacpack/internal/runtime/provider"
)

func TestApplyRefusesGPUAllWhenCDIMissing(t *testing.T) {
	bundle := buildGPUBundle(t)
	stateHome := t.TempDir()
	rec := &gpuApply{missing: true}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec, Yes: true,
	})
	if res.OK {
		t.Fatal("gpu: all without CDI must not apply")
	}
	if res.ExitHint != 6 {
		t.Fatalf("exit=%d", res.ExitHint)
	}
	var saw bool
	for _, d := range res.Diagnostics {
		if d.Code != "APPLY.GPU_UNAVAILABLE" {
			continue
		}
		saw = true
		if !strings.Contains(d.Message, "llama-chat") || !strings.Contains(d.Message, "libcuda") {
			t.Fatal(d.Message)
		}
	}
	if !saw {
		t.Fatalf("missing diagnostic: %+v", res.Diagnostics)
	}
	if len(rec.ups) != 0 {
		t.Fatalf("containers started: %v", rec.ups)
	}
}

func TestApplyCDIFailureDoesNotMaskLaterService(t *testing.T) {
	bundle := buildGPUBundle(t)
	stateHome := t.TempDir()
	rec := &gpuApply{llamaAlwaysFail: true}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec, Yes: true,
	})
	if res.OK {
		t.Fatal("expected apply to fail the llama-chat start")
	}
	var saw bool
	for _, d := range res.Diagnostics {
		if d.Code != "APPLY.GPU_UNAVAILABLE" {
			continue
		}
		saw = true
		if !strings.Contains(d.Message, "llama-chat") || !strings.Contains(d.Message, provider.NVIDIACountAllCDIDevice) {
			t.Fatal(d.Message)
		}
		if strings.Contains(d.Message, "podman-compose") {
			t.Fatal(d.Message)
		}
	}
	if !saw {
		t.Fatalf("diagnostics=%+v", res.Diagnostics)
	}
	if !hasUp(rec.ups, "zebra") {
		t.Fatalf("later service was masked: %v", rec.ups)
	}
	text := renderedCompose(t, stateHome)
	if !strings.Contains(text, "count: all") {
		t.Fatalf("gpu reservation was stripped so the service could start on CPU:\n%s", text)
	}
}

func TestApplyReadyFailureDoesNotMaskLaterService(t *testing.T) {
	bundle := buildReadyContinueBundle(t)
	stateHome := t.TempDir()
	rec := &readyFailApply{failReady: "chroma"}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec, Yes: true,
	})
	if res.OK {
		t.Fatal("expected apply to fail ready for chroma")
	}
	var sawReady bool
	for _, d := range res.Diagnostics {
		if d.Code == "APPLY.ACTION" && strings.Contains(d.Message, "chroma") {
			sawReady = true
		}
	}
	if !sawReady {
		t.Fatalf("diagnostics=%+v", res.Diagnostics)
	}
	if !hasUp(rec.ups, "zebra") || !hasUp(rec.ups, "llama-chat") {
		t.Fatalf("later services masked: ups=%v ready=%v", rec.ups, rec.ready)
	}
}

type readyFailApply struct {
	provider.RecordingApply
	failReady string
	ups       []string
	ready     []string
}

func (r *readyFailApply) ComposeUp(ctx context.Context, project, composeFile, service string, extra []string) error {
	_ = ctx
	_ = project
	_ = composeFile
	_ = extra
	r.ups = append(r.ups, service)
	return nil
}

func (r *readyFailApply) WaitReady(ctx context.Context, project, service string, timeout time.Duration) error {
	_ = ctx
	_ = project
	_ = timeout
	r.ready = append(r.ready, service)
	if service == r.failReady {
		return fmt.Errorf("readiness timeout for %s", service)
	}
	return nil
}

func buildReadyContinueBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "iacpack.yaml")
	const manifest = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    chroma:
      image: app
      healthcheck:
        command:
          - /bin/true
        interval: 10s
        timeout: 5s
        retries: 3
    llama-chat:
      image: app
      resources:
        gpu: all
    zebra:
      image: app
`
	if err := os.WriteFile(file, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "ready.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	return bundle
}

type gpuApply struct {
	provider.RecordingApply
	missing         bool
	llamaAlwaysFail bool
	ups             []string
}

func (g *gpuApply) PodmanCountAllCDIMissing() bool { return g.missing }

func (g *gpuApply) ComposeUp(ctx context.Context, project, composeFile, service string, extra []string) error {
	_ = ctx
	_ = project
	_ = composeFile
	_ = extra
	g.ups = append(g.ups, service)
	if g.llamaAlwaysFail && service == "llama-chat" {
		return &provider.CDIUnavailableError{Service: service, Device: provider.NVIDIACountAllCDIDevice}
	}
	return nil
}

func hasUp(ups []string, service string) bool {
	for _, s := range ups {
		if s == service {
			return true
		}
	}
	return false
}

func buildGPUBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "iacpack.yaml")
	const manifest = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    app:
      image: app
    llama-chat:
      image: app
      resources:
        gpu: all
    zebra:
      image: app
`
	if err := os.WriteFile(file, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "a.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	return bundle
}

func renderedCompose(t *testing.T, stateHome string) string {
	t.Helper()
	path := filepath.Join(stateHome, "projects", "example", "default", "rendered", "compose.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
