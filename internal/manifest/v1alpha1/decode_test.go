package v1alpha1_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestDecodePositive(t *testing.T) {
	root := repoRoot(t)
	var projects []domain.Project
	for _, name := range []string{"minimal.yaml", "minimal.json"} {
		data, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive", name))
		if err != nil {
			t.Fatal(err)
		}
		doc := loader.Parse(name, data)
		proj, diags := v1alpha1.Decode(doc)
		if diagnostic.HasError(diags) {
			t.Fatalf("%s: %+v", name, diags)
		}
		if proj.Metadata.Name != "example" {
			t.Fatalf("name=%q", proj.Metadata.Name)
		}
		if proj.Spec.Images["app"].Ref == "" {
			t.Fatal("missing image")
		}
		projects = append(projects, proj)
	}
	if !reflect.DeepEqual(projects[0], projects[1]) {
		t.Fatalf("YAML/JSON domain mismatch\nyaml=%#v\njson=%#v", projects[0], projects[1])
	}
}

func TestDecodeImageRefParameter(t *testing.T) {
	raw := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: development
  parameters:
    stack0-api-image:
      phase: deploy
      type: string
  images:
    app:
      ref:
        parameter: stack0-api-image
      delivery: pull
  services:
    app:
      image: app
`)
	doc := loader.Parse("draft.yaml", raw)
	proj, diags := v1alpha1.Decode(doc)
	if diagnostic.HasError(diags) {
		t.Fatalf("diags=%v", diags)
	}
	img := proj.Spec.Images["app"]
	if img.Ref != "" || img.RefParameter != "stack0-api-image" {
		t.Fatalf("image=%#v", img)
	}

	rel := bytes.Replace(raw, []byte("mode: development"), []byte("mode: release"), 1)
	doc = loader.Parse("release.yaml", rel)
	_, diags = v1alpha1.Decode(doc)
	if !hasCode(diags, diagnostic.CodeManifestMutableImageTag) {
		t.Fatalf("release diags=%v", diags)
	}
}

func TestDecodeNegatives(t *testing.T) {
	cases := []struct {
		file string
		code string
	}{
		{"unknown_field.yaml", diagnostic.CodeSchemaUnknownField},
		{"missing_required.yaml", diagnostic.CodeSchemaMissingField},
		{"bad_identifier.yaml", diagnostic.CodeSchemaInvalidIdentifier},
		{"bad_semver.yaml", diagnostic.CodeSchemaInvalidSemver},
		{"unsupported_version.yaml", diagnostic.CodeManifestUnsupportedAPI},
		{"unsupported_profile.yaml", diagnostic.CodeManifestUnsupportedProfile},
		{"mutable_tag.yaml", diagnostic.CodeManifestMutableImageTag},
	}
	dir := filepath.Join(repoRoot(t), "testdata/manifests/negative")
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			doc := loader.Parse(tc.file, data)
			_, diags := v1alpha1.Decode(doc)
			if !hasCode(diags, tc.code) {
				t.Fatalf("want %s, got %+v", tc.code, diags)
			}
		})
	}
}

func hasCode(diags []diagnostic.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
