package importpropose_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestProposeNestedImageDefaultIsParameterAndValid(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"services:",
		"  eliteew-api-worker-lan141:",
		"    image: ${STACK0_QUEUE_WORKER_IMAGE:-${STACK0_API_WORKER_IMAGE:-${STACK0_API_IMAGE}}}",
		"  eliteew-celery-worker-lan141:",
		"    image: ${STACK0_CELERY_WORKER_IMAGE:-${STACK0_API_IMAGE}}",
		"  pinned:",
		"    image: ${STACK0_PINNED_IMAGE:-${STACK0_API_IMAGE:-docker.io/library/nginx:1.27}}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir,
		Compose:   "compose.yaml",
		Name:      "eliteew",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	images := spec["images"].(map[string]any)
	for _, id := range []string{"eliteew-api-worker-lan141", "eliteew-celery-worker-lan141"} {
		img := images[id].(map[string]any)
		ref, _ := img["ref"].(map[string]any)
		if ref["parameter"] != "stack0-api-image" {
			t.Fatalf("%s ref=%#v", id, img["ref"])
		}
	}
	pinned := images["pinned"].(map[string]any)
	if pinned["ref"] != "docker.io/library/nginx:1.27" {
		t.Fatalf("pinned ref=%#v", pinned["ref"])
	}
	var fixed, gap int
	for _, d := range res.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("error %s %s", d.Code, d.Message)
		}
		if strings.Contains(d.Message, "resolved to fixed ref") {
			fixed++
			if strings.Contains(d.Message, "STACK0_QUEUE") || strings.Contains(d.Message, "STACK0_CELERY") {
				t.Fatalf("nested non-literal reported as fixed ref: %s", d.Message)
			}
		}
		if d.Code == diagnostic.CodeImportInterpolation && strings.Contains(d.Message, "mapped to parameter") {
			gap++
		}
	}
	if fixed != 1 || gap != 2 {
		t.Fatalf("fixed=%d gap=%d diags=%v", fixed, gap, res.Diagnostics)
	}
	raw, err := json.Marshal(res.Project)
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.Parse("import-draft.json", raw)
	if _, diags := v1alpha1.Decode(doc); diagnostic.HasError(diags) {
		t.Fatalf("draft invalid: %v", diags)
	}
}
