package provider

import (
	"context"
	"strings"
	"testing"

	"iacpack/internal/runtime/process"
)

func TestImageUserIDResolvesPostgresName(t *testing.T) {
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			if len(spec.Args) >= 2 && spec.Args[0] == "image" && spec.Args[1] == "inspect" {
				return process.Result{ExitCode: 0, Stdout: []byte("postgres\n")}, nil
			}
			if len(spec.Args) > 0 && spec.Args[0] == "run" {
				return process.Result{ExitCode: 0, Stdout: []byte("uid=999(postgres) gid=999(postgres) groups=999(postgres)\n")}, nil
			}
			return process.Result{ExitCode: 1}, nil
		}},
	}}
	uid, gid, ok, err := h.ImageUserID(context.Background(), "docker.io/library/postgres:18.4")
	if err != nil || !ok || uid != "999" || gid != "999" {
		t.Fatalf("uid=%s gid=%s ok=%v err=%v", uid, gid, ok, err)
	}
}

func TestImageUserIDRootDoesNotFail(t *testing.T) {
	ran := false
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			if len(spec.Args) > 0 && spec.Args[0] == "run" {
				ran = true
			}
			if len(spec.Args) >= 2 && spec.Args[0] == "image" {
				return process.Result{ExitCode: 0, Stdout: []byte("0\n")}, nil
			}
			return process.Result{ExitCode: 1}, nil
		}},
	}}
	uid, gid, ok, err := h.ImageUserID(context.Background(), "localhost/stack4-tesseract:latest")
	if err != nil || ok || uid != "" || gid != "" || ran {
		t.Fatalf("uid=%s gid=%s ok=%v err=%v ran=%v", uid, gid, ok, err, ran)
	}
}

func TestParseID(t *testing.T) {
	uid, gid, ok := parseID("uid=999(postgres) gid=999(postgres)")
	if !ok || uid != "999" || gid != "999" {
		t.Fatalf("%s %s %v", uid, gid, ok)
	}
}

func TestEnsureNetworkCreatesMissingExternalName(t *testing.T) {
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			calls = append(calls, strings.Join(spec.Args, " "))
			if len(spec.Args) >= 2 && spec.Args[0] == "network" && (spec.Args[1] == "exists" || spec.Args[1] == "inspect") {
				return process.Result{ExitCode: 1}, nil
			}
			return process.Result{ExitCode: 0}, nil
		}},
	}}
	err := h.EnsureNetwork(context.Background(), "slug", "ew_net_2847", map[string]string{"iacpack.project": "eliteew"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "network create") || !strings.Contains(joined, "ew_net_2847") {
		t.Fatalf("calls=%s", joined)
	}
	if strings.Contains(joined, ",") {
		t.Fatalf("network name must not be comma-joined: %s", joined)
	}
}

func TestEnsurePostgresDatabaseCreatesMissingName(t *testing.T) {
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			joined := strings.Join(spec.Args, " ")
			calls = append(calls, joined)
			switch {
			case strings.HasPrefix(joined, "ps -aq"):
				return process.Result{ExitCode: 0, Stdout: []byte("new\nold\n")}, nil
			case strings.HasPrefix(joined, "exec "):
				if !strings.Contains(joined, "exec new ") {
					t.Fatalf("expected the newest container: %s", joined)
				}
				return process.Result{ExitCode: 0, Stdout: []byte("DO\n")}, nil
			default:
				t.Fatalf("unexpected %s", joined)
				return process.Result{}, nil
			}
		}},
	}}
	if err := h.EnsurePostgresDatabase(context.Background(), "proj", "postgres", "eliteew", "eliteew"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "CREATE DATABASE") || !strings.Contains(joined, "'eliteew'") {
		t.Fatalf("calls=%s", joined)
	}
}
