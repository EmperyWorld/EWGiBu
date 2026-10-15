package portable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
)

func TestRunWarnsOnEmptyHostPathAndStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	manifest := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: kitwarn
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: development
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
    local:
      ref: localhost/kit:dev
      delivery: pull
  services:
    app:
      image: app
      mounts:
        - host: /data/models
          target: /models
        - host: /data/other
          target: /other
    side:
      image: local
`)
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iacpack.lock.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "kit")
	res := Run(Options{Workspace: dir, OutputDir: out})
	if !res.OK {
		t.Fatalf("kit write should succeed with warnings: %v", res.Diagnostics)
	}
	var sawHost, sawImage bool
	for _, d := range res.Diagnostics {
		if d.Code == codeHostPath {
			sawHost = true
		}
		if d.Code == codeLocalImage && strings.Contains(d.Message, "localhost/kit:dev") {
			sawImage = true
		}
		if d.Severity == "error" && (d.Code == codeHostPath || d.Code == codeLocalImage) {
			t.Fatalf("template gap must be a warning: %+v", d)
		}
	}
	if !sawHost || !sawImage {
		t.Fatalf("diags=%v", res.Diagnostics)
	}
	raw, err := os.ReadFile(filepath.Join(out, "host-paths.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "host-app-models") || !strings.Contains(text, "host-app-other") {
		t.Fatalf("parameters collided or missing: %s", text)
	}
}

func TestApplyKitHostPathsReplacesLiteralAndKeepsOperatorSet(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`[
  {"service":"app","target":"/models","parameter":"host-app-models","value":"/srv/models"},
  {"service":"app","target":"/cache","parameter":"cache-path","value":"/srv/cache"}
]
`)
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	dep := ir.Deployment{
		Services: map[string]ir.Service{
			"app": {
				Mounts: []ir.Mount{
					{HostLiteral: "/data/models", Target: "/models"},
					{HostParameter: "cache-path", Target: "/cache"},
					{Volume: "data", Target: "/var/lib"},
				},
			},
		},
	}
	ApplyKitHostPaths(&dep, dir, map[string]string{"cache-path": "/operator"})
	mounts := dep.Services["app"].Mounts
	if mounts[0].HostLiteral != "/srv/models" || mounts[0].HostParameter != "" {
		t.Fatalf("literal mount: %+v", mounts[0])
	}
	if mounts[1].HostParameter != "cache-path" || mounts[1].HostLiteral != "" {
		t.Fatalf("operator set should win: %+v", mounts[1])
	}
	if mounts[2].Volume != "data" {
		t.Fatalf("volume changed: %+v", mounts[2])
	}
}

func TestKitHostStampFollowsFilledValue(t *testing.T) {
	dir := t.TempDir()
	write := func(val string) {
		body := []byte(`[{"service":"app","target":"/models","value":"` + val + `"}]` + "\n")
		if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("/srv/models")
	a := KitHostStamp(dir)
	write("/srv/other")
	b := KitHostStamp(dir)
	if a == "" || a == b {
		t.Fatalf("stamps %s %s", a, b)
	}
	if strings.Contains(a, "/srv") || strings.Contains(b, "/srv") {
		t.Fatal("stamp leaked a path")
	}
}

func TestCheckKitRejectsRelativeHostPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), []byte(`[{"service":"app","target":"/models","parameter":"models-path","value":"relative/models"}]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images.txt"), []byte("nginx\tpull\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diags := CheckKit(dir)
	if len(diags) != 1 || diags[0].Code != CodeHostPath {
		t.Fatalf("diags=%v", diags)
	}
}
