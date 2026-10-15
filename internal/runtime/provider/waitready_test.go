package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"iacpack/internal/runtime/process"
)

func TestWaitReadyUsesEnginePSAndInspect(t *testing.T) {
	var calls []string
	rec := &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
		joined := strings.Join(spec.Args, " ")
		calls = append(calls, joined)
		switch {
		case strings.HasPrefix(joined, "ps -aq"):
			if strings.Contains(joined, "com.docker.compose.service=chroma") &&
				strings.Contains(joined, "status=running") {
				return process.Result{ExitCode: 0, Stdout: []byte("cid123\n")}, nil
			}
			return process.Result{ExitCode: 0, Stdout: []byte("")}, nil
		case strings.HasPrefix(joined, "inspect --format"):
			return process.Result{ExitCode: 0, Stdout: []byte("running healthy\n")}, nil
		default:
			t.Fatalf("unexpected argv %q", joined)
			return process.Result{}, nil
		}
	}}
	h := HostApply{Host: Host{Runner: rec, Engine: "podman", Socket: "/tmp/test.sock"}}
	if err := h.WaitReady(context.Background(), "proj", "chroma", time.Second); err != nil {
		t.Fatal(err)
	}
	if len(calls) < 2 {
		t.Fatalf("calls=%v", calls)
	}
	if strings.Contains(calls[0], "compose") && strings.Contains(calls[0], "--status") {
		t.Fatalf("must not use compose ps --status: %v", calls)
	}
	if !strings.Contains(calls[0], "ps -aq") {
		t.Fatalf("expected ps: %v", calls)
	}
}

func TestAssertWritableMountsReportsServiceAndPath(t *testing.T) {
	rec := &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
		joined := strings.Join(spec.Args, " ")
		switch {
		case strings.HasPrefix(joined, "ps -aq"):
			return process.Result{ExitCode: 0, Stdout: []byte("cid\n")}, nil
		case strings.Contains(joined, "inspect"):
			return process.Result{ExitCode: 0, Stdout: []byte("/var/lib/postgresql/data false\n/logs true\n")}, nil
		default:
			return process.Result{}, nil
		}
	}}
	h := HostApply{Host: Host{Runner: rec, Engine: "podman", Socket: "/tmp/test.sock"}}
	err := h.AssertWritableMounts(context.Background(), "proj", "postgres",
		[]string{"/var/lib/postgresql/data", "/logs"})
	var mro *MountReadOnlyError
	if !errors.As(err, &mro) {
		t.Fatalf("got %v", err)
	}
	if mro.Service != "postgres" || mro.Target != "/var/lib/postgresql/data" {
		t.Fatalf("%+v", mro)
	}
}
