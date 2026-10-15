package apply_test

import (
	"context"
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
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/provider"
)

func TestApplyTaskUser1000ReadsSecretWithoutValue(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not installed")
	}
	if exec.Command("podman", "image", "inspect", boxRef).Run() != nil {
		t.Skip("busybox image is not present locally")
	}
	const pass = "reader-secret-value"
	dir := t.TempDir()
	secret := filepath.Join(dir, "db-password")
	if err := os.WriteFile(secret, []byte(pass), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "iacpack.yaml")
	if err := os.WriteFile(manifest, []byte(secretReadYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	stderr := &strings.Builder{}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", manifest}, &strings.Builder{}, stderr, fs); code != cli.ExitOK {
		t.Fatalf("lock: %s", stderr.String())
	}
	bundle := filepath.Join(dir, "s.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", manifest, "--format", "bundle", "--output", bundle}, &strings.Builder{}, stderr, fs); code != cli.ExitOK {
		t.Fatalf("build: %s", stderr.String())
	}
	stateHome := t.TempDir()
	slug := identity.ProviderSlug("secread", "it")
	prov := provider.HostApply{Host: provider.Host{Engine: "podman"}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = destroy.Run(ctx, destroy.Options{
			Project: "secread", Instance: "it", StateHome: stateHome, Yes: true,
			Provider: provider.HostDestroy{Host: provider.Host{Engine: "podman"}},
		})
		_ = exec.Command("podman", "volume", "rm", "-f", slug+"_out").Run()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	opts := apply.Options{
		Bundle: bundle, Instance: "it", StateHome: stateHome, Yes: true,
		SecretFiles: []string{"db-password=" + secret}, Provider: prov,
	}
	res := apply.Run(ctx, opts)
	if !res.OK {
		t.Fatalf("apply: %s", strings.ReplaceAll(diagText(res), pass, "[redacted]"))
	}
	if strings.Contains(diagText(res), pass) {
		t.Fatal("secret value leaked in apply output")
	}
	got := strings.TrimSpace(unshareRead(t, slug+"_out", "result"))
	if got != "секрет прочитан" {
		t.Fatalf("result=%q", strings.ReplaceAll(got, pass, "[redacted]"))
	}
	if strings.Contains(got, pass) {
		t.Fatal("secret value written by the task")
	}
	res2 := apply.Run(ctx, opts)
	if !res2.OK || res2.Model["noop"] != true {
		t.Fatalf("second apply: ok=%v model=%v %s", res2.OK, res2.Model, strings.ReplaceAll(diagText(res2), pass, "[redacted]"))
	}
}

func unshareRead(t *testing.T, volume, name string) string {
	t.Helper()
	mp := strings.TrimSpace(execOut(t, "podman", "volume", "inspect", volume, "--format", "{{.Mountpoint}}"))
	cmd := exec.Command("podman", "unshare", "cat", filepath.Join(mp, name))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unshare cat: %v\n%s", err, out)
	}
	return string(out)
}

const secretReadYAML = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: secread
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  secrets:
    db-password:
      delivery: file
  images:
    box:
      ref: docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
      delivery: pull
  volumes:
    out:
      retention: delete
  services:
    pause:
      image: box
      command:
        - sleep
        - "3600"
  tasks:
    reader:
      phase: postDeploy
      image: box
      user: "1000"
      entrypoint:
        - /bin/sh
        - -c
      command:
        - "if [ -r /run/secrets/db ]; then printf '%s\n' 'секрет прочитан' > /out/result; else echo unread; exit 1; fi"
      secrets:
        - ref: db-password
          target: /run/secrets/db
      mounts:
        - volume: out
          target: /out
      rerun: oncePerDeployment
`
