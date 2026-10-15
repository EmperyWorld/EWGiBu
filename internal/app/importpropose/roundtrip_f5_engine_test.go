package importpropose_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/render/compose"
)

func TestF5ComposeConfigWhenEnginePresent(t *testing.T) {
	bin := ""
	for _, name := range []string{"podman", "docker"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if exec.Command(p, "compose", "version").Run() == nil {
			bin = p
			break
		}
	}
	if bin == "" {
		t.Skip("no podman/docker compose")
	}
	f := compose.File{
		Name: "f5",
		Services: map[string]compose.Service{
			"web": {
				Image:    "docker.io/library/busybox:1.36",
				Profiles: []string{"debug"},
				Deploy:   &compose.Deploy{GPUAll: true},
			},
		},
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "compose", "--profile", "debug", "-f", path, "config")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compose config: %v\n%s", err, out)
	}
	s := string(out)
	for _, part := range []string{"debug", "gpu", "busybox"} {
		if !strings.Contains(s, part) {
			t.Fatalf("config missing %q:\n%s\n--- rendered ---\n%s", part, s, raw)
		}
	}
}
