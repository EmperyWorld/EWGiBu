package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/runtime/process"
)

func TestOwnWritableUsesPodmanUnshare(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.Mkdir(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			calls = append(calls, strings.Join(spec.Args, " "))
			if len(spec.Args) >= 2 && spec.Args[0] == "volume" && spec.Args[1] == "inspect" {
				return process.Result{ExitCode: 0, Stdout: []byte(filepath.Join(dir, "vol") + "\n")}, nil
			}
			return process.Result{ExitCode: 0}, nil
		}},
	}}
	if err := os.Mkdir(filepath.Join(dir, "vol"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := h.OwnWritable(context.Background(), "proj", "1001", []string{logs}, []string{"data"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "unshare chown -R 1001:1001 "+logs) {
		t.Fatalf("calls:\n%s", joined)
	}
	if !strings.Contains(joined, "unshare chown -R 1001:1001 "+filepath.Join(dir, "vol")) {
		t.Fatalf("volume chown missing:\n%s", joined)
	}
}

func TestOwnWritableCreatesMissingPayloadDirAndChownsFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "payload", "res-elasticsearch-elasticsearch")
	file := filepath.Join(dir, "elasticsearch.yml")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			calls = append(calls, strings.Join(spec.Args, " "))
			return process.Result{ExitCode: 0}, nil
		}},
	}}
	if err := h.OwnWritable(context.Background(), "proj", "1000", []string{missing, file}, nil); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(missing)
	if err != nil || !st.IsDir() {
		t.Fatalf("missing payload dir was not created: %v", err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "unshare chown -R 1000:1000 "+missing) || !strings.Contains(joined, "unshare chown -R 1000:1000 "+file) {
		t.Fatalf("calls:\n%s", joined)
	}
}

func TestOwnWritableChmodsPayloadWhenImageUserIsEmpty(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "rendered", "payload", "res-postgres-postgres")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			calls = append(calls, strings.Join(spec.Args, " "))
			return process.Result{ExitCode: 0}, nil
		}},
	}}
	if err := h.OwnWritable(context.Background(), "proj", "", []string{logs, "/home/lady/llama-test-models"}, nil); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "unshare chmod 0777 "+logs) {
		t.Fatalf("payload chmod missing:\n%s", joined)
	}
	if strings.Contains(joined, "llama-test-models") {
		t.Fatalf("host path must not be mode 0777:\n%s", joined)
	}
}

func TestOwnSecretsChownsWithoutPrintingSecret(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "db")
	if err := os.WriteFile(secret, []byte("super-secret-value"), 0o440); err != nil {
		t.Fatal(err)
	}
	var calls []string
	h := HostApply{Host: Host{
		Engine: string(EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			calls = append(calls, strings.Join(spec.Args, " "))
			return process.Result{ExitCode: 0}, nil
		}},
	}}
	if err := h.OwnSecrets(context.Background(), "1000", []string{secret}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "unshare chown -R 1000:1000 "+secret) || !strings.Contains(joined, "unshare chmod 0440 "+secret) {
		t.Fatalf("calls:\n%s", joined)
	}
	if strings.Contains(joined, "super-secret-value") {
		t.Fatal("secret value leaked into process args")
	}
}

func TestServiceLogsKeepsTail(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "L")
	}
	got := lastLines(strings.Join(lines, "\n"), 80)
	if n := strings.Count(got, "\n") + 1; n != 80 {
		t.Fatalf("lines=%d", n)
	}
}
