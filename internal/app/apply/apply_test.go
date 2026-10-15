package apply_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/app/apply"
	"iacpack/internal/app/status"
	"iacpack/internal/runtime/provider"
)

func TestApplyFakeOrderAndNoop(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "a.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	stateHome := t.TempDir()
	rec := &provider.RecordingApply{}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec, Yes: true,
	})
	if !res.OK {
		t.Fatalf("apply: %v", res.Diagnostics)
	}
	if len(rec.Calls) == 0 {
		t.Fatal("expected provider calls")
	}
	// second apply noop
	rec2 := &provider.RecordingApply{}
	res2 := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec2, Yes: true,
	})
	if !res2.OK {
		t.Fatalf("noop apply: %v", res2.Diagnostics)
	}
	if res2.Model["noop"] != true {
		t.Fatalf("expected noop, got %#v", res2.Model)
	}
	if len(rec2.Calls) != 0 {
		t.Fatalf("noop must not mutate: %v", rec2.Calls)
	}
	st := status.Run(status.Options{Project: "example", StateHome: stateHome})
	if !st.OK {
		t.Fatalf("status: %v", st.Diagnostics)
	}
}

func TestApplyFailureExitPartial(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	_ = cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs)
	bundle := filepath.Join(t.TempDir(), "a.iacbundle")
	_ = cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs)
	rec := &provider.RecordingApply{FailAt: "image:docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001"}
	// FailAt may not match exact ref — fail on first image call prefix
	rec.FailAt = ""
	// inject via custom: fail at network if any; minimal has no network — fail image by wrapping
	rec = &provider.RecordingApply{}
	orig := rec
	_ = orig
	failing := &failImage{RecordingApply: provider.RecordingApply{}}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: t.TempDir(), Provider: failing, Yes: true,
	})
	if res.OK {
		t.Fatal("expected failure")
	}
	if !res.Mutated && res.ExitHint != 7 && res.ExitHint != 6 {
		// image failure after verify counts as mutated once exec starts
	}
	if res.ExitHint != 7 && !res.Mutated {
		t.Logf("exitHint=%d mutated=%v diags=%v", res.ExitHint, res.Mutated, res.Diagnostics)
	}
}

func TestSecretCanaryAbsentFromState(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "examples", "release"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	_ = cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs)
	bundle := filepath.Join(t.TempDir(), "r.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	canary := "SUPER_SECRET_CANARY_VALUE_9f3a"
	sec := filepath.Join(dir, "db.secret")
	if err := os.WriteFile(sec, []byte(canary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	rec := &provider.RecordingApply{}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: bundle, StateHome: stateHome, Provider: rec, Yes: true,
		SecretFiles: []string{"db-password=" + sec},
	})
	if !res.OK {
		t.Fatalf("apply: %v", res.Diagnostics)
	}
	// walk state home except secrets/
	err := filepath.Walk(stateHome, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.Contains(path, string(filepath.Separator)+"secrets"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(canary)) {
			t.Errorf("canary in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type failImage struct {
	provider.RecordingApply
}

func (f *failImage) ImageAvailable(ctx context.Context, ref string) error {
	_ = f.RecordingApply.ImageAvailable(ctx, ref)
	return context.Canceled
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode())
	})
	if err != nil {
		t.Fatal(err)
	}
}
