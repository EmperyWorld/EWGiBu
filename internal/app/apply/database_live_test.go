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

const (
	pgRef  = "docker.io/library/postgres@sha256:724292da1f2e50bdccfc3302ce75bbba7f4a6076701b588cc795fcac65683550"
	boxRef = "docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
	dbPass = "s3cret-dbextra"
)

func TestApplySecondDatabaseOnExistingPostgres(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not installed")
	}
	if exec.Command("podman", "image", "inspect", pgRef).Run() != nil || exec.Command("podman", "image", "inspect", boxRef).Run() != nil {
		t.Skip("postgres or busybox image is not present locally")
	}

	dir := t.TempDir()
	sqlDir := filepath.Join(dir, "sql", "ewa")
	if err := os.MkdirAll(sqlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sql := "CREATE TABLE IF NOT EXISTS ewa_item (id int primary key, name text);\nINSERT INTO ewa_item (id, name) VALUES (1, 'ok') ON CONFLICT (id) DO NOTHING;\n"
	if err := os.WriteFile(filepath.Join(sqlDir, "001_init.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "db-password")
	if err := os.WriteFile(secret, []byte(dbPass), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "iacpack.yaml")
	if err := os.WriteFile(manifest, []byte(databaseProjectYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := filesystem.OS{}
	lockErr := &strings.Builder{}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", manifest}, &strings.Builder{}, lockErr, fs); code != cli.ExitOK {
		t.Fatalf("lock: %s", lockErr.String())
	}
	bundle := filepath.Join(dir, "db.iacbundle")
	stderr := &strings.Builder{}
	if code := cli.ExecuteWithFS([]string{"build", "--file", manifest, "--format", "bundle", "--output", bundle}, &strings.Builder{}, stderr, fs); code != cli.ExitOK {
		t.Fatalf("build: %s", stderr.String())
	}

	stateHome := t.TempDir()
	slug := identity.ProviderSlug("dbextra", "it")
	prov := provider.HostApply{Host: provider.Host{Engine: "podman"}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = destroy.Run(ctx, destroy.Options{
			Project: "dbextra", Instance: "it", StateHome: stateHome, Yes: true,
			Provider: provider.HostDestroy{Host: provider.Host{Engine: "podman"}},
		})
		_ = exec.Command("podman", "volume", "rm", "-f", slug+"_db-data", slug+"_marker").Run()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res := apply.Run(ctx, apply.Options{
		Bundle: bundle, Instance: "it", StateHome: stateHome, Yes: true,
		SecretFiles: []string{"db-password=" + secret}, Provider: prov,
	})
	if !res.OK {
		rendered := filepath.Join(stateHome, "projects", "dbextra", "it", "rendered", "compose.yaml")
		if b, err := os.ReadFile(rendered); err == nil {
			_ = os.WriteFile("/tmp/db-compose.yaml", b, 0o644)
		}
		t.Fatalf("apply: %s", redactPass(diagText(res)))
	}
	if strings.Contains(res.Text, dbPass) || strings.Contains(diagText(res), dbPass) {
		t.Fatal("secret value leaked in apply output")
	}

	name := psql(t, slug, "SELECT name FROM ewa_item")
	if strings.TrimSpace(name) != "ok" {
		t.Fatalf("ewa_item=%q", redactPass(name))
	}
	dsn := strings.TrimSpace(execOut(t, "podman", "exec", containerID(t, slug, "api", true), "printenv", "DATABASE_URL"))
	if !strings.HasPrefix(dsn, "postgresql://postgres:") || !strings.HasSuffix(dsn, "@postgres:5432/ewa") {
		t.Fatal("dsn shape mismatch")
	}
	running := execOut(t, "podman", "ps", "--filter", "label=com.docker.compose.project="+slug, "--format", "{{.Label \"com.docker.compose.service\"}}")
	if strings.Count(running, "postgres") != 1 || strings.Contains(running, "db-ewa") {
		t.Fatalf("running services:\n%s", running)
	}

	res2 := apply.Run(ctx, apply.Options{
		Bundle: bundle, Instance: "it", StateHome: stateHome, Yes: true,
		SecretFiles: []string{"db-password=" + secret}, Provider: prov,
	})
	if !res2.OK || res2.Model["noop"] != true {
		t.Fatalf("second apply: ok=%v model=%v %s", res2.OK, res2.Model, redactPass(diagText(res2)))
	}

	markerBefore := strings.TrimSpace(volumeFile(t, slug+"_marker", "count"))
	if err := os.WriteFile(filepath.Join(sqlDir, "001_init.sql"), []byte(sql+"INSERT INTO ewa_item (id, name) VALUES (2, 'edited') ON CONFLICT (id) DO NOTHING;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle2 := filepath.Join(dir, "db2.iacbundle")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", manifest}, &strings.Builder{}, lockErr, fs); code != cli.ExitOK {
		t.Fatalf("relock: %s", lockErr.String())
	}
	if code := cli.ExecuteWithFS([]string{"build", "--file", manifest, "--format", "bundle", "--output", bundle2}, &strings.Builder{}, stderr, fs); code != cli.ExitOK {
		t.Fatalf("rebuild: %s", stderr.String())
	}
	res3 := apply.Run(ctx, apply.Options{
		Bundle: bundle2, Instance: "it", StateHome: stateHome, Yes: true,
		SecretFiles: []string{"db-password=" + secret}, Provider: prov,
	})
	if !res3.OK || res3.Model["noop"] == true {
		t.Fatalf("sql change should reapply: ok=%v model=%v %s", res3.OK, res3.Model, redactPass(diagText(res3)))
	}
	markerAfter := strings.TrimSpace(volumeFile(t, slug+"_marker", "count"))
	if markerBefore == "" || markerBefore != markerAfter {
		t.Fatalf("marker task reran: before=%q after=%q", markerBefore, markerAfter)
	}
	edited := psql(t, slug, "SELECT name FROM ewa_item WHERE id = 2")
	if strings.TrimSpace(edited) != "edited" {
		t.Fatalf("edited sql was not applied: %q", redactPass(edited))
	}
}

func psql(t *testing.T, slug, query string) string {
	t.Helper()
	id := containerID(t, slug, "postgres", true)
	return execOut(t, "podman", "exec", id, "psql", "-U", "postgres", "-d", "ewa", "-tAc", query)
}

func containerID(t *testing.T, slug, service string, running bool) string {
	t.Helper()
	args := []string{"ps", "-q", "--filter", "label=com.docker.compose.project=" + slug, "--filter", "label=com.docker.compose.service=" + service}
	if running {
		args = append(args, "--filter", "status=running")
	}
	id := strings.TrimSpace(execOut(t, "podman", args...))
	if id == "" {
		args = []string{"ps", "-q", "--filter", "label=io.podman.compose.project=" + slug, "--filter", "label=io.podman.compose.service=" + service}
		if running {
			args = append(args, "--filter", "status=running")
		}
		id = strings.TrimSpace(execOut(t, "podman", args...))
	}
	if id == "" {
		t.Fatalf("no container %s in %s", service, slug)
	}
	if i := strings.IndexByte(id, '\n'); i >= 0 {
		id = id[:i]
	}
	return id
}

func volumeFile(t *testing.T, volume, name string) string {
	t.Helper()
	mp := strings.TrimSpace(execOut(t, "podman", "volume", "inspect", volume, "--format", "{{.Mountpoint}}"))
	b, err := os.ReadFile(filepath.Join(mp, name))
	if err != nil {
		t.Fatalf("read %s/%s: %v", volume, name, err)
	}
	return string(b)
}

func execOut(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, redactPass(string(out)))
	}
	return string(out)
}

func diagText(res apply.Result) string {
	var b strings.Builder
	b.WriteString(res.Text)
	for _, d := range res.Diagnostics {
		b.WriteString(d.Message)
		b.WriteByte('\n')
	}
	return b.String()
}

func redactPass(s string) string {
	return strings.ReplaceAll(s, dbPass, "[redacted]")
}

const databaseProjectYAML = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: dbextra
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  secrets:
    db-password:
      delivery: file
  images:
    pg:
      ref: docker.io/library/postgres@sha256:724292da1f2e50bdccfc3302ce75bbba7f4a6076701b588cc795fcac65683550
      delivery: pull
    box:
      ref: docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
      delivery: pull
  networks:
    backend:
  volumes:
    db-data:
      retention: retain
    marker:
      retention: delete
  services:
    postgres:
      image: pg
      environment:
        POSTGRES_USER:
          literal: postgres
        POSTGRES_PASSWORD:
          secret: db-password
      secrets:
        - ref: db-password
          target: /run/secrets/db-password
      mounts:
        - volume: db-data
          target: /var/lib/postgresql/data
      networks:
        - backend
      healthcheck:
        command:
          - pg_isready
          - -U
          - postgres
        interval: 5s
        timeout: 3s
        retries: 20
    api:
      image: box
      command:
        - sleep
        - "3600"
      environment:
        DATABASE_URL:
          parameter: ewa-database-url
      networks:
        - backend
  databases:
    ewa:
      on: postgres
      name: ewa
      ownerFrom: postgres
      migrations: sql/ewa
      urlParameter: ewa-database-url
  tasks:
    marker:
      phase: postDeploy
      image: box
      entrypoint:
        - /bin/sh
        - -c
      command:
        - echo ran >> /marker/count
      mounts:
        - volume: marker
          target: /marker
      rerun: oncePerDeployment
`
