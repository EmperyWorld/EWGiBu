package v1alpha1_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/policy"
	"iacpack/internal/schemaembed"
)

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

func decodeFile(t *testing.T, rel string) (diags []diagnostic.Diagnostic, ok bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.Parse(filepath.Base(rel), data)
	_, diags = v1alpha1.Decode(doc)
	return diags, !diagnostic.HasError(diags)
}

func TestExamplesValidate(t *testing.T) {
	root := filepath.Join(repoRoot(t), "examples")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base != "iacpack.yaml" && base != "iacpack.json" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot(t), path)
		if err != nil {
			return err
		}
		t.Run(rel, func(t *testing.T) {
			diags, ok := decodeFile(t, rel)
			if !ok {
				t.Fatalf("decode failed: %v", diags)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := loader.Parse(base, data)
			proj, _ := v1alpha1.Decode(doc)
			pdiags := policy.Evaluate(policy.MustDefault(), proj)
			if diagnostic.HasError(pdiags) {
				t.Fatalf("policy: %v", pdiags)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPositiveCorpus(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata/manifests/positive")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			diags, ok := decodeFile(t, filepath.Join("testdata/manifests/positive", name))
			if !ok {
				t.Fatalf("expected ok, diags=%v", diags)
			}
			pol := policy.MustDefault()
			// union_arms exercises external network/volume arms; default policy forbids them.
			if strings.HasPrefix(name, "union_arms") {
				pol.Capabilities.ExternalNetworks = true
				pol.Capabilities.ExternalVolumes = true
			}
			docData, _ := os.ReadFile(filepath.Join(root, name))
			doc := loader.Parse(name, docData)
			proj, _ := v1alpha1.Decode(doc)
			pdiags := policy.Evaluate(pol, proj)
			if diagnostic.HasError(pdiags) {
				t.Fatalf("policy: %v", pdiags)
			}
		})
	}
}

func TestNegativeCorpus(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata/manifests/negative")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			diags, ok := decodeFile(t, filepath.Join("testdata/manifests/negative", name))
			// Some negatives are policy-only (evaluated after decode).
			if strings.HasPrefix(name, "policy_") {
				if !ok {
					// decode may also fail; still run policy when possible
					return
				}
				docData, _ := os.ReadFile(filepath.Join(root, name))
				doc := loader.Parse(name, docData)
				proj, d := v1alpha1.Decode(doc)
				if diagnostic.HasError(d) {
					t.Fatalf("policy fixture should decode: %v", d)
				}
				pdiags := policy.Evaluate(policy.MustDefault(), proj)
				if !diagnostic.HasError(pdiags) {
					t.Fatal("expected policy error")
				}
				return
			}
			if name == "package_option_conflict.yaml" {
				docData, _ := os.ReadFile(filepath.Join(root, name))
				doc := loader.Parse(name, docData)
				proj, d := v1alpha1.Decode(doc)
				if diagnostic.HasError(d) {
					t.Fatalf("decode: %v", d)
				}
				pdiags := policy.Evaluate(policy.MustDefault(), proj)
				if !diagnostic.HasError(pdiags) {
					t.Fatal("expected package option conflict via policy")
				}
				return
			}
			if ok {
				t.Fatalf("expected failure, got ok diags=%v", diags)
			}
		})
	}
}

func TestYAMLJSONEquivalence(t *testing.T) {
	y, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/positive/complete_release.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/positive/complete_release.json"))
	if err != nil {
		t.Fatal(err)
	}
	py, dy := v1alpha1.Decode(loader.Parse("y.yaml", y))
	pj, dj := v1alpha1.Decode(loader.Parse("j.json", j))
	if diagnostic.HasError(dy) || diagnostic.HasError(dj) {
		t.Fatalf("decode errors yaml=%v json=%v", dy, dj)
	}
	hy, err := v1alpha1.SemanticHash(py)
	if err != nil {
		t.Fatal(err)
	}
	hj, err := v1alpha1.SemanticHash(pj)
	if err != nil {
		t.Fatal(err)
	}
	if hy != hj {
		t.Fatalf("hash mismatch yaml=%s json=%s", hy, hj)
	}
}

func TestSemanticHashIgnoresXUI(t *testing.T) {
	base, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/positive/complete_release.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	moved := bytes.ReplaceAll(base, []byte("x: 480"), []byte("x: 999"))
	moved = bytes.ReplaceAll(moved, []byte("y: 120"), []byte("y: 1"))
	p1, d1 := v1alpha1.Decode(loader.Parse("a.yaml", base))
	p2, d2 := v1alpha1.Decode(loader.Parse("b.yaml", moved))
	if diagnostic.HasError(d1) || diagnostic.HasError(d2) {
		t.Fatalf("decode %v %v", d1, d2)
	}
	h1, _ := v1alpha1.SemanticHash(p1)
	h2, _ := v1alpha1.SemanticHash(p2)
	if h1 != h2 {
		t.Fatalf("x-ui move changed hash: %s vs %s", h1, h2)
	}
}

func TestSchemaEmbedDrift(t *testing.T) {
	root := repoRoot(t)
	want, err := os.ReadFile(filepath.Join(root, "schemas/manifest/v1alpha1/schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := schemaembed.ManifestV1Alpha1()
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatal("embedded manifest schema differs from schemas/manifest/v1alpha1/schema.json")
	}
	wantP, err := os.ReadFile(filepath.Join(root, "schemas/policy/v1/schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	gotP := schemaembed.PolicyV1()
	if !bytes.Equal(bytes.TrimSpace(wantP), bytes.TrimSpace(gotP)) {
		t.Fatal("embedded policy schema differs from schemas/policy/v1/schema.json")
	}
}

func TestSecretCanaryNotInDiagnosticsOrInspect(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/negative/secret_value.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(v1alpha1.SecretCanaryMarker)) {
		t.Fatal("fixture missing canary")
	}
	doc := loader.Parse("secret_value.yaml", data)
	_, diags := v1alpha1.Decode(doc)
	blob, _ := json.Marshal(diagnostic.ToMaps(diags))
	if bytes.Contains(blob, []byte(v1alpha1.SecretCanaryMarker)) {
		t.Fatal("canary leaked into diagnostics")
	}
	// Positive fixture with secret declaration only — inspect must not invent values.
	pos, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/positive/complete_release.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	proj, d := v1alpha1.Decode(loader.Parse("ok.yaml", pos))
	if diagnostic.HasError(d) {
		t.Fatal(d)
	}
	model, err := v1alpha1.PublicInspectModel(proj)
	if err != nil {
		t.Fatal(err)
	}
	mb, _ := json.Marshal(model)
	if bytes.Contains(mb, []byte(v1alpha1.SecretCanaryMarker)) {
		t.Fatal("canary in inspect")
	}
	text, _ := v1alpha1.FormatInspectText(proj)
	if strings.Contains(text, v1alpha1.SecretCanaryMarker) {
		t.Fatal("canary in inspect text")
	}
}

func TestPackageFormatConflict(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/negative/package_option_conflict.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	proj, d := v1alpha1.Decode(loader.Parse("p.yaml", data))
	if diagnostic.HasError(d) {
		t.Fatal(d)
	}
	diags := policy.PackageFormatConflict(proj.Spec.Package, "directory")
	if !diagnostic.HasError(diags) {
		t.Fatal("expected format conflict")
	}
}

func TestInspectManifestReorderStable(t *testing.T) {
	base, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/manifests/positive/complete_release.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Reorder top-level and nested map keys without changing semantics.
	reordered := []byte(`kind: Project
apiVersion: iacpack.io/v1alpha1
metadata:
  version: 1.4.0
  name: acme-shop
  description: Acme Shop deployment
spec:
  mode: release
  target:
    profile: compose-docker/v1
  secrets:
    db-password:
      delivery: file
      required: true
      description: Database password
  parameters:
    db-user:
      type: string
      phase: deploy
      default: acme
    http-port:
      maximum: 65535
      minimum: 1
      type: integer
      phase: deploy
      default: 8080
      description: Loopback HTTP port
  images:
    postgres:
      delivery: pull
      ref: docker.io/library/postgres@sha256:2222222222222222222222222222222222222222222222222222222222222222
    app:
      delivery: pull
      ref: ghcr.io/acme/shop@sha256:1111111111111111111111111111111111111111111111111111111111111111
  volumes:
    db-data:
      retention: retain
  networks:
    backend:
  resources:
    app-config:
      mode: "0644"
      destination: config
      include:
        - app.yaml
      source: config
  services:
    app:
      restart: unless-stopped
      image: app
      resources:
        memory: 268435456
        cpus: "1"
      healthcheck:
        retries: 20
        timeout: 3s
        interval: 5s
        command:
          - /app/healthcheck
      networks:
        - backend
      mounts:
        - readOnly: true
          target: /etc/acme/app.yaml
          source: app.yaml
          resource: app-config
      ports:
        - protocol: tcp
          bind: 127.0.0.1
          host:
            parameter: http-port
          container: 8080
      dependsOn:
        - condition: healthy
          target: db
    db:
      image: postgres
      healthcheck:
        retries: 20
        timeout: 3s
        interval: 5s
        command:
          - pg_isready
          - -U
          - acme
      networks:
        - backend
      mounts:
        - target: /var/lib/postgresql/data
          volume: db-data
      secrets:
        - target: /run/secrets/db-password
          ref: db-password
      environment:
        POSTGRES_PASSWORD_FILE:
          literal: /run/secrets/db-password
        POSTGRES_USER:
          parameter: db-user
        POSTGRES_DB:
          literal: acme
  tasks:
    migrate:
      timeout: 5m
      rerun: oncePerDeployment
      dependsOn:
        - condition: healthy
          target: db
      command:
        - /app/migrate
      image: app
      phase: migration
  package:
    generateEnvExample: true
    generateReadme: true
    embedImages: false
    includeRuntime: false
  x-ui:
    viewport:
      zoom: 1
      y: 0
      x: 0
    nodes:
      db:
        category: database
        y: 120
        x: 120
      app:
        category: app
        y: 120
        x: 480
`)
	p1, d1 := v1alpha1.Decode(loader.Parse("base.yaml", base))
	p2, d2 := v1alpha1.Decode(loader.Parse("reordered.yaml", reordered))
	if diagnostic.HasError(d1) || diagnostic.HasError(d2) {
		t.Fatalf("decode base=%v reordered=%v", d1, d2)
	}
	h1, err := v1alpha1.SemanticHash(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := v1alpha1.SemanticHash(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("semantic hash unstable under key reorder: %s vs %s", h1, h2)
	}
	m1, err := v1alpha1.PublicInspectModel(p1)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := v1alpha1.PublicInspectModel(p2)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(m1)
	b2, _ := json.Marshal(m2)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("inspect JSON unstable under key reorder\n%s\nvs\n%s", b1, b2)
	}
	t1, err := v1alpha1.FormatInspectText(p1)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := v1alpha1.FormatInspectText(p2)
	if err != nil {
		t.Fatal(err)
	}
	if t1 != t2 {
		t.Fatalf("inspect text unstable under key reorder:\n%q\nvs\n%q", t1, t2)
	}
	// Repeated runs must be identical.
	for i := 0; i < 5; i++ {
		again, err := v1alpha1.PublicInspectModel(p1)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(again)
		if !bytes.Equal(b, b1) {
			t.Fatalf("inspect JSON unstable across repeated runs (i=%d)", i)
		}
	}
}

func TestSchemaGoStructuralDrift(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(schemaembed.ManifestV1Alpha1(), &schema); err != nil {
		t.Fatal(err)
	}
	defs, _ := schema["$defs"].(map[string]any)
	if defs == nil {
		t.Fatal("schema missing $defs")
	}

	mustEnum := func(def, prop string, want []string) {
		t.Helper()
		d, _ := defs[def].(map[string]any)
		props, _ := d["properties"].(map[string]any)
		p, _ := props[prop].(map[string]any)
		raw, _ := p["enum"].([]any)
		got := make([]string, 0, len(raw))
		for _, v := range raw {
			got = append(got, v.(string))
		}
		sort.Strings(got)
		wantSorted := append([]string(nil), want...)
		sort.Strings(wantSorted)
		if strings.Join(got, ",") != strings.Join(wantSorted, ",") {
			t.Fatalf("%s.%s enum drift: schema=%v go=%v", def, prop, got, wantSorted)
		}
	}

	mustEnum("parameter", "phase", []string{string(domain.ParameterPhaseBuild), string(domain.ParameterPhaseDeploy)})
	mustEnum("parameter", "type", []string{
		string(domain.ParameterTypeString), string(domain.ParameterTypeInteger),
		string(domain.ParameterTypeNumber), string(domain.ParameterTypeBoolean),
		string(domain.ParameterTypeEnum),
	})
	mustEnum("dependency", "condition", []string{"started", "healthy", "completedSuccessfully"})
	mustEnum("task", "phase", []string{
		string(domain.TaskPhasePreStart), string(domain.TaskPhaseMigration), string(domain.TaskPhasePostDeploy),
	})
	mustEnum("task", "rerun", []string{"always", "oncePerDeployment", "manual"})
	mustEnum("service", "restart", []string{"no", "always", "on-failure", "unless-stopped"})
	mustEnum("port", "protocol", []string{"tcp", "udp"})

	volDef, _ := defs["volume"].(map[string]any)
	volArms, _ := volDef["oneOf"].([]any)
	if len(volArms) < 2 {
		t.Fatal("volume oneOf missing arms")
	}
	managedVol, _ := volArms[0].(map[string]any)
	retEnum, _ := managedVol["properties"].(map[string]any)["retention"].(map[string]any)["enum"].([]any)
	retGot := make([]string, 0, len(retEnum))
	for _, v := range retEnum {
		retGot = append(retGot, v.(string))
	}
	sort.Strings(retGot)
	if strings.Join(retGot, ",") != "delete,retain" {
		t.Fatalf("volume.retention enum drift: %v", retGot)
	}
	// Mode / profile / apiVersion consts
	props, _ := schema["properties"].(map[string]any)
	specProps, _ := props["spec"].(map[string]any)["properties"].(map[string]any)
	modeEnum, _ := specProps["mode"].(map[string]any)["enum"].([]any)
	modes := []string{}
	for _, v := range modeEnum {
		modes = append(modes, v.(string))
	}
	sort.Strings(modes)
	wantModes := []string{string(domain.ModeDevelopment), string(domain.ModeRelease)}
	sort.Strings(wantModes)
	if strings.Join(modes, ",") != strings.Join(wantModes, ",") {
		t.Fatalf("mode enum drift: %v vs %v", modes, wantModes)
	}
	apiConst, _ := props["apiVersion"].(map[string]any)["const"].(string)
	if apiConst != domain.APIVersionV1Alpha1 {
		t.Fatalf("apiVersion const drift: %q vs %q", apiConst, domain.APIVersionV1Alpha1)
	}
	kindConst, _ := props["kind"].(map[string]any)["const"].(string)
	if kindConst != domain.KindProject {
		t.Fatalf("kind const drift: %q vs %q", kindConst, domain.KindProject)
	}
	profileConst, _ := specProps["target"].(map[string]any)["properties"].(map[string]any)["profile"].(map[string]any)["const"].(string)
	if profileConst != string(domain.ProfileComposeDockerV1) {
		t.Fatalf("profile const drift: %q vs %q", profileConst, domain.ProfileComposeDockerV1)
	}

	// Defaults applied by decode must match schema defaults.
	secretProps, _ := defs["secret"].(map[string]any)["properties"].(map[string]any)
	if secretProps["required"].(map[string]any)["default"] != true {
		t.Fatal("secret.required schema default must be true")
	}
	if secretProps["delivery"].(map[string]any)["default"] != "file" {
		t.Fatal("secret.delivery schema default must be file")
	}
	svcRestartDef, _ := defs["service"].(map[string]any)["properties"].(map[string]any)["restart"].(map[string]any)["default"].(string)
	if svcRestartDef != "unless-stopped" {
		t.Fatalf("service.restart default drift: %q", svcRestartDef)
	}
	pkgProps, _ := defs["package"].(map[string]any)["properties"].(map[string]any)
	for _, pair := range []struct {
		field string
		want  bool
	}{
		{"generateReadme", true},
		{"generateEnvExample", true},
		{"includeRuntime", false},
		{"embedImages", false},
	} {
		got, _ := pkgProps[pair.field].(map[string]any)["default"].(bool)
		if got != pair.want {
			t.Fatalf("package.%s default drift: %v want %v", pair.field, got, pair.want)
		}
	}

	// Discriminated unions present as oneOf with expected arms.
	for _, def := range []string{"image", "network", "volume", "mount", "valueSource"} {
		d, ok := defs[def].(map[string]any)
		if !ok {
			t.Fatalf("missing $defs.%s", def)
		}
		arms, ok := d["oneOf"].([]any)
		if !ok || len(arms) < 2 {
			t.Fatalf("$defs.%s must be a oneOf with >=2 arms", def)
		}
	}
}

func FuzzDecode(f *testing.F) {
	dir, err := os.Getwd()
	if err != nil {
		f.Fatal(err)
	}
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			f.Fatal("go.mod not found")
		}
		root = parent
	}
	_ = filepath.Walk(filepath.Join(root, "testdata/manifests"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err == nil && len(b) < 64<<10 {
			f.Add(b)
		}
		return nil
	})
	f.Add([]byte("{"))
	f.Add([]byte("a: ["))
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
		}()
		doc := loader.Parse("fuzz.yaml", data)
		_, _ = v1alpha1.Decode(doc)
	})
}
