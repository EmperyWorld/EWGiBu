package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/manifest/loader"
)

func TestParseComposeInputOverrideTag(t *testing.T) {
	raw := []byte("services:\n  web:\n    image: nginx:1\n    command: !override\n      - /bin/app\n    volumes: !reset\n")
	doc := loader.ParseComposeInput("override.yml", raw)
	for _, d := range doc.Diags {
		if d.Code == "MANIFEST.CUSTOM_TAG_FORBIDDEN" {
			t.Fatalf("compose merge tag refused: %s", d.Message)
		}
	}
	if len(doc.Diags) > 0 {
		t.Fatalf("diags: %v", doc.Diags)
	}
	cmd := doc.Root.GetMap("services").GetMap("web").GetMap("command")
	if cmd == nil || cmd.Kind != loader.KindSeq || len(cmd.Seq) != 1 || cmd.Tag != "override" {
		t.Fatalf("command=%v", cmd)
	}
	vols := doc.Root.GetMap("services").GetMap("web").GetMap("volumes")
	if vols == nil || vols.Tag != "reset" {
		t.Fatalf("volumes=%v", vols)
	}
}

func TestParseManifestStillForbidsOverrideTag(t *testing.T) {
	raw := []byte("spec:\n  command: !override\n    - /bin/app\n")
	doc := loader.Parse("iacpack.yaml", raw)
	found := false
	for _, d := range doc.Diags {
		if d.Code == "MANIFEST.CUSTOM_TAG_FORBIDDEN" {
			found = true
		}
	}
	if !found {
		t.Fatal("manifest parser must still forbid !override")
	}
}

func TestParseComposeInputFlowNetworks(t *testing.T) {
	raw := []byte("services:\n  web:\n    image: nginx:1\n    networks: [a, b]\n")
	doc := loader.ParseComposeInput("c.yml", raw)
	if len(doc.Diags) > 0 {
		t.Fatalf("diags: %v", doc.Diags)
	}
	nets := doc.Root.GetMap("services").GetMap("web").GetMap("networks")
	if nets == nil || nets.Kind != loader.KindSeq || len(nets.Seq) != 2 {
		t.Fatalf("networks=%v", nets)
	}
}

func TestParseComposeInputEnvInterpolationBraces(t *testing.T) {
	raw := []byte("services:\n  app:\n    image: nginx:1\n    environment:\n      - FOO=${FOO:?required}\n")
	doc := loader.ParseComposeInput("c.yml", raw)
	if len(doc.Diags) > 0 {
		t.Fatalf("diags: %v", doc.Diags)
	}
}

func TestParseManifestStillRejectsFlow(t *testing.T) {
	raw := []byte("services:\n  web:\n    networks: [a]\n")
	doc := loader.Parse("c.yml", raw)
	if len(doc.Diags) == 0 {
		t.Fatal("expected manifest parse error")
	}
}

func TestParseComposeInputFlowSeqTrailingComma(t *testing.T) {
	raw := []byte("services:\n  app:\n    image: nginx:1\n    command:\n      [\n        \"run\",\n        \"--port\",\n        \"8000\",\n      ]\n")
	doc := loader.ParseComposeInput("c.yml", raw)
	if len(doc.Diags) > 0 {
		t.Fatalf("diags: %v", doc.Diags)
	}
	cmd := doc.Root.GetMap("services").GetMap("app").GetMap("command")
	if cmd == nil || cmd.Kind != loader.KindSeq || len(cmd.Seq) != 3 {
		t.Fatalf("command=%v", cmd)
	}
}

func TestParseComposeInputStack1Fixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "ui", "two-service", "stack1.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.ParseComposeInput("stack1.yml", raw)
	if len(doc.Diags) > 0 {
		for _, d := range doc.Diags {
			t.Errorf("%s line %d col %d: %s", d.Code, d.Source.Line, d.Source.Column, d.Message)
		}
		t.Fatal("parse failed")
	}
	if doc.Root.GetMap("services") == nil {
		t.Fatal("no services")
	}
}

func TestParseComposeInputStack4Fixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "ui", "two-service", "stack4.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("stack4.yml fixture not in workspace")
	}
	doc := loader.ParseComposeInput("stack4.yml", raw)
	if len(doc.Diags) > 0 {
		for _, d := range doc.Diags {
			t.Errorf("%s line %d col %d: %s", d.Code, d.Source.Line, d.Source.Column, d.Message)
		}
		t.Fatal("parse failed")
	}
	if doc.Root.GetMap("services") == nil {
		t.Fatal("no services")
	}
}
