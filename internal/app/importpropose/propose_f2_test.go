package importpropose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func TestProposeBareImageInterpolationKeepsService(t *testing.T) {
	dir := t.TempDir()
	body := []byte(strings.Join([]string{
		"services:",
		"  eliteew-api-prod:",
		"    image: ${STACK0_API_IMAGE}",
		"  baked:",
		"    image: ${STACK4_API_BAKED_IMAGE:?Set STACK4_API_BAKED_IMAGE}",
		"  web:",
		"    image: docker.io/library/nginx:1.27",
		"",
	}, "\n"))
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	override := []byte(strings.Join([]string{
		"services:",
		"  web:",
		"    command: !override",
		"      - /bin/app",
		"",
	}, "\n"))
	if err := os.WriteFile(filepath.Join(dir, "override.yml"), override, 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Composes:  []string{"compose.yaml", "override.yml"},
		Name:      "eliteew",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	if res.Summary.Services != 3 {
		t.Fatalf("services=%d", res.Summary.Services)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	if _, ok := services["eliteew-api-prod"]; !ok {
		t.Fatal("missing eliteew-api-prod")
	}
	if _, ok := services["baked"]; !ok {
		t.Fatal("missing baked")
	}
	var gapBare, gapReq bool
	for _, g := range res.ParameterGaps {
		if g.ID == "stack0-api-image" && g.Reason == "no-default" {
			gapBare = true
		}
		if g.ID == "stack4-api-baked-image" && g.Reason == "no-default" {
			gapReq = true
		}
	}
	if !gapBare || !gapReq {
		t.Fatalf("gaps=%v", res.ParameterGaps)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected error %s %s", d.Code, d.Message)
		}
	}
}

func TestProposeF2PartialOKPolicyWarning(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f2")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "f2",
	})
	if !res.OK {
		t.Fatalf("expected ok=true partial draft, diags=%v", res.Diagnostics)
	}
	if res.Summary.Services != 1 {
		t.Fatalf("services=%d want 1 (web only)", res.Summary.Services)
	}
	var policyWarn, profileInfo int
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportPolicyDenied && d.Severity == diagnostic.SeverityWarning {
			policyWarn++
		}
		if d.Code == diagnostic.CodeImportProfileSkipped {
			profileInfo++
		}
	}
	if policyWarn != 1 {
		t.Fatalf("policy warnings=%d", policyWarn)
	}
	if profileInfo != 1 {
		t.Fatalf("profile skipped infos=%d", profileInfo)
	}
}

func TestProposeF2ActiveProfiles(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f2")
	res := importpropose.Propose(importpropose.Request{
		Workspace:      ws,
		Compose:        "compose.yaml",
		Name:           "f2",
		ActiveProfiles: []string{"debug"},
	})
	if !res.OK || res.Summary.Services != 2 {
		t.Fatalf("ok=%v services=%d", res.OK, res.Summary.Services)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	debug := services["debug"].(map[string]any)
	profs, _ := debug["profiles"].([]any)
	if len(profs) != 1 || profs[0] != "debug" {
		t.Fatalf("profiles=%v", debug["profiles"])
	}
}

func TestProposeF2StrictPolicyError(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f2")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "f2",
		Strict:    true,
	})
	if res.OK {
		t.Fatal("expected ok=false with import strict when gpu service denied")
	}
}

func TestProposeStack2NoImageInterpolationErrors(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "ui", "two-service")
	if _, err := os.Stat(filepath.Join(ws, "stack2.yml")); err != nil {
		t.Skip("stack2.yml not present")
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "stack2.yml",
		Name:      "stack2",
	})
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportInterpolation && d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected image error: %s", d.Message)
		}
	}
}

func TestProposeStack2WithProfileImportsToolRouter(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "ui", "two-service")
	if _, err := os.Stat(filepath.Join(ws, "stack2.yml")); err != nil {
		t.Skip("stack2.yml not present")
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace:      ws,
		Compose:        "stack2.yml",
		Name:           "stack2",
		ActiveProfiles: []string{"companion-tool-router"},
	})
	if !res.OK {
		t.Fatalf("expected ok, diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	if _, ok := services["llama-tool-router"]; !ok {
		t.Fatal("expected llama-tool-router with profile active")
	}
}
