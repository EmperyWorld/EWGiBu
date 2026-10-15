package apply

import (
	"context"
	"strings"
	"testing"
	"time"

	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/process"
	"iacpack/internal/runtime/provider"
)

func TestImageWorkDirsKeepsExplicitTaskDir(t *testing.T) {
	h := provider.HostApply{Host: provider.Host{
		Engine: string(provider.EnginePodman),
		Runner: &process.RecordingRunner{Script: func(spec process.Spec) (process.Result, error) {
			if strings.Contains(strings.Join(spec.Args, " "), "WorkingDir") {
				return process.Result{ExitCode: 0, Stdout: []byte("/app\n")}, nil
			}
			return process.Result{ExitCode: 1}, nil
		}},
	}}
	dep := ir.Deployment{
		Images: map[string]ir.Image{"tesseract": {Ref: "localhost/stack4-tesseract:latest"}},
		Services: map[string]ir.Service{
			"tesseract": {Image: "tesseract"},
		},
		Tasks: map[string]ir.Task{
			"keep": {Image: "tesseract", Workdir: "/elsewhere", Command: ir.LiteralArgs("/bin/true")},
		},
	}
	got, err := imageWorkDirs(context.Background(), h, dep)
	if err != nil {
		t.Fatal(err)
	}
	if got["tesseract"] != "/app" {
		t.Fatalf("service workdir: %v", got)
	}
	if _, ok := got["keep"]; ok {
		t.Fatalf("explicit task workdir was overwritten: %v", got)
	}
}

func TestTaskRunTimeoutUsesManifest(t *testing.T) {
	if taskRunTimeout("30m") != 30*time.Minute {
		t.Fatal(taskRunTimeout("30m"))
	}
	if taskRunTimeout("") != 5*time.Minute {
		t.Fatal("empty timeout")
	}
}
