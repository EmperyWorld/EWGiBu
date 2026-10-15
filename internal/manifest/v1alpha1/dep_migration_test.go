package v1alpha1_test

import (
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestServiceMayDependOnMigrationCompletedSuccessfully(t *testing.T) {
	doc := loader.Parse("migrate-dep.yaml", []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
    db:
      ref: docker.io/library/postgres@sha256:0000000000000000000000000000000000000000000000000000000000000002
      delivery: pull
  services:
    postgres:
      image: db
      healthcheck:
        command:
          - pg_isready
        interval: 5s
        timeout: 3s
        retries: 5
    api:
      image: app
      dependsOn:
        - target: sql-migrate
          condition: completedSuccessfully
  tasks:
    sql-migrate:
      phase: migration
      image: db
      command:
        - /migrations/pack-migrate
      dependsOn:
        - target: postgres
          condition: healthy
`))
	_, diags := v1alpha1.Decode(doc)
	if diagnostic.HasError(diags) {
		t.Fatalf("expected ok: %v", diags)
	}
}

func TestServiceMigrationRequiresCompletedSuccessfully(t *testing.T) {
	doc := loader.Parse("bad-cond.yaml", []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    api:
      image: app
      dependsOn:
        - target: sql-migrate
          condition: started
  tasks:
    sql-migrate:
      phase: migration
      image: app
      command:
        - /migrations/pack-migrate
`))
	_, diags := v1alpha1.Decode(doc)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected error")
	}
	var saw bool
	for _, d := range diags {
		if d.Code == diagnostic.CodeManifestInvalidDependency && strings.Contains(d.Message, "completedSuccessfully") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("diags=%v", diags)
	}
}

func TestServiceCannotDependOnPostDeploy(t *testing.T) {
	doc := loader.Parse("postdeploy.yaml", []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    api:
      image: app
      dependsOn:
        - target: after
          condition: completedSuccessfully
  tasks:
    after:
      phase: postDeploy
      image: app
      command:
        - /bin/true
`))
	_, diags := v1alpha1.Decode(doc)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected error")
	}
	var saw bool
	for _, d := range diags {
		if d.Code == diagnostic.CodeManifestInvalidDependency && strings.Contains(d.Message, "postDeploy") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("diags=%v", diags)
	}
}
