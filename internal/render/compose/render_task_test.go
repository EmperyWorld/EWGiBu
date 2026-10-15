package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
)

func TestRenderTasksAsProfiledServices(t *testing.T) {
	ref := "docker.io/library/busybox@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images: map[string]ir.Image{
			"app": {Ref: ref},
		},
		Services: map[string]ir.Service{
			"web": {Image: "app", Restart: "unless-stopped"},
		},
		Tasks: map[string]ir.Task{
			"seed": {
				Phase: "preStart", Image: "app", Command: ir.LiteralArgs("/seed"),
				Rerun: "manual",
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{Lock: lockfile.Document{
		Images: map[string]lockfile.ImageLock{
			"app": {Ref: ref},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	seed, ok := f.Services["seed"]
	if !ok {
		t.Fatal("seed task missing from compose services")
	}
	if len(seed.Profiles) != 1 || seed.Profiles[0] != "iacpack-task" {
		t.Fatalf("profiles=%v", seed.Profiles)
	}
	if seed.Restart != "no" {
		t.Fatalf("restart=%q", seed.Restart)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "iacpack-task") {
		t.Fatalf("yaml missing profile:\n%s", raw)
	}
	if _, ok := f.Services["web"]; !ok {
		t.Fatal("web service dropped")
	}
}

func TestRenderTaskEntrypointAndUser(t *testing.T) {
	ref := "quay.io/keycloak/keycloak@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"kc": {Ref: ref}},
		Tasks: map[string]ir.Task{
			"eliteew-keycloak-realm": {
				Phase:      "postDeploy",
				Image:      "kc",
				User:       "0",
				Entrypoint: ir.LiteralArgs("/bin/sh"),
				Command:    ir.LiteralArgs("/realm/keycloak-pack-realm-bootstrap.sh"),
				Rerun:      "oncePerDeployment",
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{Lock: lockfile.Document{
		Images: map[string]lockfile.ImageLock{"kc": {Ref: ref}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	svc := f.Services["eliteew-keycloak-realm"]
	if svc.User != "0" {
		t.Fatalf("user=%q", svc.User)
	}
	if len(svc.Entrypoint) != 1 || svc.Entrypoint[0] != "/bin/sh" {
		t.Fatalf("entrypoint=%v", svc.Entrypoint)
	}
	if len(svc.Command) != 1 || svc.Command[0] != "/realm/keycloak-pack-realm-bootstrap.sh" {
		t.Fatalf("command=%v", svc.Command)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "user: \"0\"") && !strings.Contains(text, "user: 0\n") {
		t.Fatalf("missing user:\n%s", text)
	}
	if !strings.Contains(text, "entrypoint:") || !strings.Contains(text, "/bin/sh") {
		t.Fatalf("missing entrypoint:\n%s", text)
	}
	cleared := d
	task := cleared.Tasks["eliteew-keycloak-realm"]
	task.Entrypoint = nil
	task.ResetEntrypoint = true
	cleared.Tasks["eliteew-keycloak-realm"] = task
	f, err = compose.Render(cleared, compose.RenderOptions{Lock: lockfile.Document{
		Images: map[string]lockfile.ImageLock{"kc": {Ref: ref}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "entrypoint: []") {
		t.Fatalf("empty entrypoint not cleared:\n%s", raw)
	}
}

func TestHostBindRelabelsForSELinux(t *testing.T) {
	ref := "ghcr.io/ggml-org/llama.cpp:server-cuda13"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"llama": {Ref: ref}},
		Services: map[string]ir.Service{
			"llama-chat": {
				Image: "llama",
				User:  "0:0",
				Mounts: []ir.Mount{{
					HostParameter: "stack2-llama-models-path",
					Target:        "/models",
					ReadOnly:      true,
				}},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		DeployValues: map[string]string{"stack2-llama-models-path": "/home/lady/llama-test-models"},
		Lock:         lockfile.Document{Images: map[string]lockfile.ImageLock{"llama": {Ref: ref}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "selinux: z") || !strings.Contains(text, "/models") {
		t.Fatalf("host model bind missing selinux label:\n%s", text)
	}
}

func TestImageWorkDirIsRendered(t *testing.T) {
	ref := "localhost/stack4-tesseract:latest"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"tesseract": {Ref: ref}},
		Services: map[string]ir.Service{
			"tesseract": {
				Image:      "tesseract",
				Entrypoint: ir.LiteralArgs("/usr/local/bin/log-to-file.sh"),
				Command:    ir.LiteralArgs("/logs/tesseract.log", "uvicorn", "server:app"),
			},
		},
		Tasks: map[string]ir.Task{
			"keep": {Image: "tesseract", Workdir: "/elsewhere", Command: ir.LiteralArgs("/bin/true")},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		Lock:          lockfile.Document{Images: map[string]lockfile.ImageLock{"tesseract": {Ref: ref}}},
		ImageWorkDirs: map[string]string{"tesseract": "/app", "keep": "/app", "nope": "../etc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "working_dir: /app") || !strings.Contains(text, "cd /app && exec") {
		t.Fatalf("image workdir not pinned:\n%s", text)
	}
	if !strings.Contains(text, "/usr/local/bin/log-to-file.sh") || !strings.Contains(text, "server:app") {
		t.Fatalf("original entrypoint dropped:\n%s", text)
	}
	if !strings.Contains(text, "working_dir: /elsewhere") {
		t.Fatalf("task workdir replaced:\n%s", text)
	}
	if strings.Contains(text, "..") {
		t.Fatalf("unsafe workdir rendered:\n%s", text)
	}
}
