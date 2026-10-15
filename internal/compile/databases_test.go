package compile_test

import (
	"strings"
	"testing"

	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
)

func TestExpandDatabasesAddsMigrationTaskAndDSN(t *testing.T) {
	user := domain.Scalar{Kind: domain.ScalarString, Str: "postgres"}
	p := domain.Project{
		Metadata: domain.Metadata{Name: "demo", Version: "0.1.0"},
		Spec: domain.Spec{
			Mode:    domain.ModeRelease,
			Target:  domain.Target{Profile: domain.ProfileComposeDockerV1},
			Images:  map[string]domain.Image{"pg": {Ref: "docker.io/library/postgres@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Delivery: "pull"}},
			Secrets: map[string]domain.Secret{"db-password": {Required: true, Delivery: "file"}},
			Services: map[string]domain.Service{
				"postgres": {
					Image: "pg",
					Environment: map[string]domain.ValueSource{
						"POSTGRES_USER":     {Literal: &user},
						"POSTGRES_PASSWORD": {Secret: "db-password"},
					},
					Networks: []string{"backend"},
					Healthcheck: &domain.Healthcheck{
						Command: []string{"pg_isready"}, Interval: "5s", Timeout: "3s", Retries: 5,
					},
				},
				"api": {
					Image: "pg",
					Environment: map[string]domain.ValueSource{
						"DATABASE_URL": {Parameter: "ewa-database-url"},
					},
				},
			},
			Parameters: map[string]domain.Parameter{
				"ewa-database-url": {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeString},
			},
			Databases: map[string]domain.Database{
				"ewa": {
					On: "postgres", Name: "ewa", OwnerFrom: "postgres",
					Migrations: "sql/ewa", URLParameter: "ewa-database-url",
				},
			},
			Networks: map[string]domain.Network{"backend": {}},
		},
	}

	d, err := compile.ToIR(p)
	if err != nil {
		t.Fatal(err)
	}
	task, ok := d.Tasks["db-ewa"]
	if !ok {
		t.Fatalf("tasks=%v", d.Tasks)
	}
	if task.Phase != "migration" || task.Rerun != "oncePerDeployment" {
		t.Fatalf("%+v", task)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0].Target != "postgres" || task.DependsOn[0].Condition != "healthy" {
		t.Fatalf("deps=%v", task.DependsOn)
	}
	env := d.Services["api"].Environment["DATABASE_URL"]
	if len(env.Parts) == 0 {
		t.Fatalf("dsn not rewritten: %+v", env)
	}
	joined := ""
	for _, part := range env.Parts {
		if part.Literal != nil {
			joined += part.Literal.(string)
		}
		if part.Secret != "" {
			joined += "<secret:" + part.Secret + ">"
		}
	}
	if !strings.Contains(joined, "@postgres:5432/ewa") || !strings.Contains(joined, "<secret:db-password>") {
		t.Fatalf("dsn=%q", joined)
	}
	g, gdiags := compile.BuildGraph(d)
	if diagnostic.HasError(gdiags) {
		t.Fatalf("graph %v", gdiags)
	}
	var sawTask bool
	for _, id := range g.Order {
		if id == "action:task/db-ewa" {
			sawTask = true
		}
	}
	if !sawTask {
		t.Fatalf("order=%v", g.Order)
	}
	script := compile.DatabaseMigrateScript("postgres", "postgres", "ewa", false, true)
	if !strings.Contains(script, "CREATE DATABASE") || !strings.Contains(script, "iacpack_schema_migrations") {
		t.Fatalf("script=%s", script)
	}
	if strings.Contains(script, "secret-value") {
		t.Fatal("secret value leaked")
	}
}
