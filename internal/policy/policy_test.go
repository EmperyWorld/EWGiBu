package policy_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/policy"
)

func TestDefaultPolicyEmbedMatchesCheckedIn(t *testing.T) {
	root := repoRoot(t)
	want, err := os.ReadFile(filepath.Join(root, "policies/default.compile.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "internal/policy/default.compile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatal("policies/default.compile.json drifts from internal/policy/default.compile.json")
	}
	pol, err := policy.Default()
	if err != nil {
		t.Fatal(err)
	}
	if pol.Ceilings.CPUs == "" || pol.Ceilings.MemoryBytes == 0 {
		t.Fatalf("default ceilings unset: %+v", pol.Ceilings)
	}
}

func TestEvaluateResourceCeilings(t *testing.T) {
	pol := policy.MustDefault()
	pol.Ceilings = policy.ResourceCeilings{CPUs: "1", MemoryBytes: 64 << 20}

	proj := domain.Project{Spec: domain.Spec{
		Mode: domain.ModeRelease,
		Images: map[string]domain.Image{
			"app": {Ref: "docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001", Delivery: "pull"},
		},
		Services: map[string]domain.Service{
			"app": {
				Image: "app",
				Resources: &domain.ResourceLimits{
					CPUs:   "2",
					Memory: 128 << 20,
				},
			},
		},
	}}
	diags := policy.Evaluate(pol, proj)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected resource ceiling errors")
	}
	var cpu, mem bool
	for _, d := range diags {
		if d.Code != diagnostic.CodePolicyResourceCeiling {
			continue
		}
		if bytes.Contains([]byte(d.Message), []byte("cpus")) {
			cpu = true
		}
		if bytes.Contains([]byte(d.Message), []byte("memory")) {
			mem = true
		}
	}
	if !cpu || !mem {
		t.Fatalf("want cpus+memory ceiling diags, got %#v", diags)
	}

	ok := domain.Project{Spec: domain.Spec{
		Mode: domain.ModeRelease,
		Images: map[string]domain.Image{
			"app": {Ref: "docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001", Delivery: "pull"},
		},
		Services: map[string]domain.Service{
			"app": {
				Image: "app",
				Resources: &domain.ResourceLimits{
					CPUs:   "0.5",
					Memory: 32 << 20,
				},
			},
		},
	}}
	if diagnostic.HasError(policy.Evaluate(pol, ok)) {
		t.Fatal("within ceilings should pass")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
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
