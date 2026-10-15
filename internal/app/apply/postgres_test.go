package apply

import (
	"testing"

	"iacpack/internal/domain/ir"
)

func TestHostBindSourcesKeepsReadOnlyModels(t *testing.T) {
	svc := ir.Service{Mounts: []ir.Mount{{
		HostParameter: "stack2-llama-models-path",
		Target:        "/models",
		ReadOnly:      true,
	}}}
	got := hostBindSources(svc, map[string]string{"stack2-llama-models-path": "/home/lady/llama-test-models"})
	if len(got) != 1 || got[0] != "/home/lady/llama-test-models" {
		t.Fatal(got)
	}
}

func TestPostgresIdentity(t *testing.T) {
	svc := ir.Service{Environment: map[string]ir.ValueSource{
		"POSTGRES_USER": {Parameter: "postgres-user"},
		"POSTGRES_DB":   {Parameter: "postgres-db"},
	}}
	user, db, ok := postgresIdentity(svc, map[string]string{"postgres-user": "eliteew", "postgres-db": "eliteew"})
	if !ok || user != "eliteew" || db != "eliteew" {
		t.Fatalf("%s %s %v", user, db, ok)
	}
	if _, _, ok = postgresIdentity(svc, map[string]string{"postgres-user": "eliteew", "postgres-db": "bad name"}); ok {
		t.Fatal("database name with a space was accepted")
	}
}

func TestPostgresServerImageIgnoresAPI(t *testing.T) {
	dep := ir.Deployment{Images: map[string]ir.Image{
		"api": {Ref: "localhost/eliteew-api:prod-baked-20260928"},
		"pg":  {Ref: "docker.io/library/postgres:18.4"},
		"alp": {Ref: "docker.io/library/postgres:18.4-alpine"},
	}}
	if postgresServerImage(dep, ir.Service{Image: "api"}) {
		t.Fatal("api image must not be treated as the database server")
	}
	if !postgresServerImage(dep, ir.Service{Image: "pg"}) || !postgresServerImage(dep, ir.Service{Image: "alp"}) {
		t.Fatal("official postgres images were not recognized")
	}
}
