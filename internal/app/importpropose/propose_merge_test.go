package importpropose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func TestProposeOverrideFragmentBeforeBaseReplacesCommand(t *testing.T) {
	dir := writeMergeCompose(t,
		"override.yml", strings.Join([]string{
			"services:",
			"  llama-chat:",
			"    command: !override",
			"      - --model",
			"      - gemma",
			"",
		}, "\n"),
		"base.yml", strings.Join([]string{
			"services:",
			"  web:",
			"    image: docker.io/library/nginx:1.27",
			"  llama-chat:",
			"    image: docker.io/library/nginx:1.27",
			"    command:",
			"      - --model",
			"      - base",
			"",
		}, "\n"),
	)
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Composes:  []string{"override.yml", "base.yml"},
		Name:      "merge",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	svc := spec["services"].(map[string]any)["llama-chat"].(map[string]any)
	cmd, _ := svc["command"].([]any)
	if len(cmd) != 2 || cmd[0] != "--model" || cmd[1] != "gemma" {
		t.Fatalf("command=%v", svc["command"])
	}
	if svc["image"] != "llama-chat" {
		t.Fatalf("image=%v", svc["image"])
	}
	assertNoEmptyService(t, res.Diagnostics)
}

func TestProposeOverrideFragmentAfterBaseReplacesCommand(t *testing.T) {
	dir := writeMergeCompose(t,
		"base.yml", strings.Join([]string{
			"services:",
			"  llama-chat:",
			"    image: docker.io/library/nginx:1.27",
			"    command:",
			"      - --model",
			"      - base",
			"",
		}, "\n"),
		"override.yml", strings.Join([]string{
			"services:",
			"  llama-chat:",
			"    command: !override",
			"      - --model",
			"      - gemma",
			"",
		}, "\n"),
	)
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Composes:  []string{"base.yml", "override.yml"},
		Name:      "merge",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	svc := spec["services"].(map[string]any)["llama-chat"].(map[string]any)
	cmd, _ := svc["command"].([]any)
	if len(cmd) != 2 || cmd[1] != "gemma" {
		t.Fatalf("command=%v", svc["command"])
	}
	assertNoEmptyService(t, res.Diagnostics)
}

func TestProposeDevicesBaseDropsFragmentWithoutEmptyError(t *testing.T) {
	dir := writeMergeCompose(t,
		"override.yml", strings.Join([]string{
			"services:",
			"  llama-chat:",
			"    command: !override",
			"      - --model",
			"      - gemma",
			"",
		}, "\n"),
		"base.yml", strings.Join([]string{
			"services:",
			"  web:",
			"    image: docker.io/library/nginx:1.27",
			"  llama-chat:",
			"    image: docker.io/library/nginx:1.27",
			"    devices:",
			"      - /dev/kvm",
			"",
		}, "\n"),
	)
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Composes:  []string{"override.yml", "base.yml"},
		Name:      "merge",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	if _, ok := services["llama-chat"]; ok {
		t.Fatal("devices base must still exclude llama-chat")
	}
	if _, ok := services["web"]; !ok {
		t.Fatal("missing web")
	}
	assertNoEmptyService(t, res.Diagnostics)
}

func TestProposeAbsoluteBindIsWarningUnlessStrict(t *testing.T) {
	dir := writeMergeCompose(t,
		"compose.yaml", strings.Join([]string{
			"services:",
			"  api:",
			"    image: docker.io/library/nginx:1.27",
			"    volumes:",
			"      - /mnt/eliteew-prod-line-export/api:/data",
			"      - ./rel:/work",
			"",
		}, "\n"),
	)
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Compose:   "compose.yaml",
		Name:      "binds",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	svc := spec["services"].(map[string]any)["api"].(map[string]any)
	mounts, _ := svc["mounts"].([]any)
	if len(mounts) != 1 {
		t.Fatalf("mounts=%v", svc["mounts"])
	}
	var warn bool
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportPolicyDenied && d.Severity == diagnostic.SeverityWarning {
			warn = true
		}
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected error %s %s", d.Code, d.Message)
		}
	}
	if !warn {
		t.Fatal("expected absolute bind warning")
	}
	strict := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Compose:   "compose.yaml",
		Name:      "binds",
		Strict:    true,
	})
	if strict.OK {
		t.Fatal("expected ok=false with --import-strict")
	}
	if _, ok := strict.Project["spec"].(map[string]any)["services"].(map[string]any)["api"]; !ok {
		t.Fatal("strict mode dropped the service")
	}
}

func TestProposeVolumeOverrideDropsAbsoluteAndReplacesBase(t *testing.T) {
	dir := writeMergeCompose(t,
		"base.yml", strings.Join([]string{
			"services:",
			"  api:",
			"    image: docker.io/library/nginx:1.27",
			"    volumes:",
			"      - ./keep:/keep",
			"",
		}, "\n"),
		"override.yml", strings.Join([]string{
			"services:",
			"  api:",
			"    volumes: !override",
			"      - /mnt/eliteew-prod-line-export/api:/data",
			"",
		}, "\n"),
	)
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Composes:  []string{"override.yml", "base.yml"},
		Name:      "binds",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	svc := spec["services"].(map[string]any)["api"].(map[string]any)
	if _, ok := svc["mounts"]; ok {
		t.Fatalf("absolute override kept mounts=%v", svc["mounts"])
	}
	assertNoEmptyService(t, res.Diagnostics)
}

func writeMergeCompose(t *testing.T, nameBody ...string) string {
	t.Helper()
	if len(nameBody)%2 != 0 {
		t.Fatal("name/body pairs required")
	}
	dir := t.TempDir()
	for i := 0; i < len(nameBody); i += 2 {
		if err := os.WriteFile(filepath.Join(dir, nameBody[i]), []byte(nameBody[i+1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func assertNoEmptyService(t *testing.T, diags []diagnostic.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Code == diagnostic.CodeImportEmpty && d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected empty diagnostic: %s", d.Message)
		}
	}
}
