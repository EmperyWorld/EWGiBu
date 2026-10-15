package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/app/apply"
	"iacpack/internal/app/destroy"
	"iacpack/internal/runtime/provider"
)

func TestApplyKitListsImageAndPathThenPasses(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not installed")
	}
	const probe = "localhost/kit-probe:dev"
	if exec.Command("podman", "image", "inspect", "docker.io/library/busybox:1.36").Run() != nil {
		t.Skip("busybox:1.36 is not present locally")
	}
	if err := exec.Command("podman", "tag", "docker.io/library/busybox:1.36", probe).Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("podman", "rmi", probe).Run()
	})

	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policyPath, []byte(kitProbePolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), []byte(kitProbeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	stderr := &bytes.Buffer{}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", filepath.Join(dir, "iacpack.yaml"), "--policy", policyPath}, &bytes.Buffer{}, stderr, fs); code != cli.ExitOK {
		t.Fatalf("lock %d %s", code, stderr.String())
	}
	kit := t.TempDir()
	stderr.Reset()
	code := cli.ExecuteWithFS([]string{
		"bundle", "--workspace", dir, "--output-dir", kit, "--policy", policyPath, "--embed-local-images",
	}, &bytes.Buffer{}, stderr, fs)
	if _, err := os.Stat(filepath.Join(kit, "project.iacbundle")); err != nil {
		t.Fatalf("bundle %d missing project.iacbundle: %s", code, stderr.String())
	}
	images, err := os.ReadFile(filepath.Join(kit, "images.txt"))
	if err != nil {
		t.Fatalf("bundle %d did not write images.txt: %s", code, stderr.String())
	}
	var tarRel string
	for _, line := range strings.Split(string(images), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 && parts[1] == "load" && parts[0] == probe {
			tarRel = parts[2]
		}
	}
	if tarRel == "" {
		t.Fatalf("images.txt missing load line:\n%s", images)
	}
	tarPath := filepath.Join(kit, filepath.FromSlash(tarRel))
	if st, err := os.Stat(tarPath); err != nil || st.Size() == 0 {
		t.Fatalf("oci archive: %v", err)
	}
	secrets, _ := os.ReadFile(filepath.Join(kit, "secrets.txt"))
	if bytes.Contains(secrets, []byte("reader-secret-value")) || bytes.Contains(secrets, []byte("s3cret")) {
		t.Fatal("secret value in kit")
	}

	if err := exec.Command("podman", "rmi", probe).Run(); err != nil {
		t.Fatal(err)
	}
	stdout, stderr2 := &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{"apply", kit}, stdout, stderr2, fs)
	if code == cli.ExitOK {
		t.Fatalf("apply should stop\n%s\n%s", stdout.String(), stderr2.String())
	}
	body := stderr2.String()
	if !strings.Contains(body, "PORTABLE.LOCAL_IMAGE") || !strings.Contains(body, probe) {
		t.Fatalf("missing image diagnostic:\n%s", body)
	}
	if !strings.Contains(body, "PORTABLE.HOST_PATH") || !strings.Contains(body, "models-path") {
		t.Fatalf("missing path diagnostic:\n%s", body)
	}

	if out, err := exec.Command("podman", "load", "-i", tarPath).CombinedOutput(); err != nil {
		t.Fatalf("load: %v\n%s", err, out)
	}
	hostsPath := filepath.Join(kit, "host-paths.json")
	raw, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("host paths: %s", raw)
	}
	models := t.TempDir()
	rows[0]["value"] = models
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostsPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	prov := provider.HostApply{Host: provider.Host{Engine: "podman"}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = destroy.Run(ctx, destroy.Options{
			Project: "kitprobe", Instance: "it", StateHome: stateHome, Yes: true,
			Provider: provider.HostDestroy{Host: provider.Host{Engine: "podman"}},
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res := apply.Run(ctx, apply.Options{
		Bundle: kit, Instance: "it", StateHome: stateHome, Yes: true, Provider: prov,
	})
	if !res.OK {
		t.Fatalf("apply: %s", diagTextKit(res))
	}
	id := strings.TrimSpace(execOutKit(t, "podman", "ps", "-q",
		"--filter", "label=com.docker.compose.service=app",
		"--filter", "status=running"))
	if id == "" {
		t.Fatal("app container is not running")
	}
}

func execOutKit(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	return string(out)
}

func diagTextKit(res apply.Result) string {
	var b strings.Builder
	b.WriteString(res.Text)
	for _, d := range res.Diagnostics {
		b.WriteString(d.Message)
		b.WriteByte('\n')
	}
	return b.String()
}

const kitProbeYAML = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: kitprobe
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: development
  parameters:
    models-path:
      phase: deploy
      type: string
  images:
    app:
      ref: localhost/kit-probe:dev
      delivery: pull
  services:
    app:
      image: app
      command:
        - sleep
        - "30"
      mounts:
        - host:
            parameter: models-path
          target: /models
`

const kitProbePolicy = `{
  "apiVersion": "iacpack.policy/v1",
  "kind": "CompilePolicy",
  "metadata": {"name": "kit-probe"},
  "spec": {
    "registries": {"allow": ["docker.io", "localhost"]},
    "capabilities": {
      "developmentBuilds": true,
      "buildNetwork": false,
      "externalNetworks": false,
      "externalVolumes": false,
      "includeRuntime": false,
      "embedImages": false
    },
    "quotas": {
      "maxEntries": 50000,
      "maxEntryBytes": 4294967296,
      "maxExpandedBytes": 21474836480,
      "maxCompressionRatio": 200,
      "maxPathBytes": 255
    },
    "resourceCeilings": {"cpus": "1024", "memoryBytes": 1099511627776}
  }
}
`
