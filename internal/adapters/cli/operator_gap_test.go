package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
)

func TestPlanStopsWhenDraftIsNewerThanBundleLock(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "m.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	updated := bytes.Replace(raw, []byte("version: 0.1.0"), []byte("version: 0.1.1"), 1)
	if bytes.Equal(raw, updated) {
		t.Fatal("version was not replaced")
	}
	if err := os.WriteFile(file, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"plan", bundle, "--workspace", dir}, stdout, stderr, fs)
	if code == cli.ExitOK {
		t.Fatalf("plan should stop\n%s\n%s", stdout.String(), stderr.String())
	}
	text := stderr.String()
	if !strings.Contains(text, "lock устарел") || !strings.Contains(text, "iacpack diff --from") {
		t.Fatalf("stderr=%s", text)
	}
}

func TestExplainServiceJSON(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "m.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"explain", bundle, "app", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("explain %d %s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "explain.schema.json")
	if !bytes.Contains(stdout.Bytes(), []byte(`"gpu":"none"`)) && !bytes.Contains(stdout.Bytes(), []byte(`"gpu": "none"`)) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestBundleEmptyHostPathWarnsAndWritesArchive(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("    app:\n      image: app\n"), []byte("    app:\n      image: app\n      mounts:\n        - host: /data/models\n          target: /models\n"), 1)
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	kit := filepath.Join(t.TempDir(), "kit")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"bundle", "--workspace", dir, "--output-dir", kit, "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("empty host path is a warning\n%s\n%s", stdout.String(), stderr.String())
	}
	body := stdout.String() + stderr.String()
	hasWarn := strings.Contains(body, `"severity":"warning"`) || strings.Contains(body, `"severity": "warning"`)
	if !strings.Contains(body, "PORTABLE.HOST_PATH") || !hasWarn {
		t.Fatalf("body=%s", body)
	}
	if _, err := os.Stat(filepath.Join(kit, "project.iacbundle")); err != nil {
		t.Fatal(err)
	}
}

func TestBundleKitOmitsSecretsAndFlagsLocalImage(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("mode: release"), []byte("mode: development"), 1)
	raw = bytes.Replace(raw, []byte("docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001"), []byte("localhost/eliteew-api:dev"), 1)
	raw = bytes.Replace(raw, []byte("    app:\n      image: app\n"), []byte("    app:\n      image: app\n      mounts:\n        - host: /data/models\n          target: /models\n"), 1)
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(t.TempDir(), "kit")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"bundle", "--workspace", dir, "--output-dir", kit, "--output", "json"}, stdout, stderr, fs)
	if code == cli.ExitOK {
		t.Fatalf("local image and host path should fail\n%s\n%s", stdout.String(), stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "bundle.schema.json")
	body := stdout.String() + stderr.String()
	if !strings.Contains(body, "PORTABLE.LOCAL_IMAGE") || !strings.Contains(body, "PORTABLE.HOST_PATH") {
		t.Fatalf("body=%s", body)
	}
	values, err := os.ReadFile(filepath.Join(kit, "values.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(values, []byte("localhost/eliteew-api")) {
		t.Fatal("image tag leaked into values")
	}
	secrets, err := os.ReadFile(filepath.Join(kit, "secrets.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(secrets, []byte("SUPER_SECRET")) {
		t.Fatal("secret value in kit")
	}
	images, err := os.ReadFile(filepath.Join(kit, "images.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(images, []byte("local-only")) {
		t.Fatalf("images=%s", images)
	}
}
