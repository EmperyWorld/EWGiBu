package apply

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/bundle"
	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/provider"
)

func TestOnlyChangedSQLHashMoves(t *testing.T) {
	db := ir.Task{
		Rerun:  "oncePerDeployment",
		Phase:  "migration",
		Mounts: []ir.Mount{{Resource: "db-ewa", Target: "/iacpack/db", ReadOnly: true}},
	}
	other := ir.Task{
		Rerun:   "oncePerDeployment",
		Phase:   "migration",
		Command: ir.LiteralArgs("/bin/true"),
	}
	entries := map[string]bundle.Entry{
		"payload/db-ewa/sql/001.sql": {Path: "payload/db-ewa/sql/001.sql", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	hDB := migrationInputHash(db, entries, nil)
	hOther := migrationInputHash(other, entries, nil)
	entries["payload/db-ewa/sql/001.sql"] = bundle.Entry{
		Path: "payload/db-ewa/sql/001.sql", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if migrationInputHash(db, entries, nil) == hDB {
		t.Fatal("sql byte change did not move the database task hash")
	}
	if migrationInputHash(other, entries, nil) != hOther {
		t.Fatal("unrelated task hash moved")
	}
}

func TestMigrationHashFollowsMountBytes(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "migrate.sh")
	if err := os.WriteFile(script, []byte("create table v1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := ir.Task{
		Phase:   "migration",
		Rerun:   "oncePerDeployment",
		User:    "0",
		Command: ir.LiteralArgs("/bin/sh", "/migrate.sh"),
		Mounts:  []ir.Mount{{HostLiteral: script, Target: "/migrate.sh", ReadOnly: true}},
	}
	h1 := migrationInputHash(task, nil, nil)
	if err := os.WriteFile(script, []byte("create table v2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2 := migrationInputHash(task, nil, nil)
	if h1 == "" || h1 == h2 {
		t.Fatalf("hash did not change: %s %s", h1, h2)
	}
	task.Command = ir.LiteralArgs("/bin/sh", "/other.sh")
	h3 := migrationInputHash(task, nil, nil)
	if h3 == h2 {
		t.Fatal("command change did not change hash")
	}
}

type logStub struct{ provider.RecordingApply }

func (s *logStub) ServiceLogs(ctx context.Context, project, service string, lines int) (string, error) {
	_ = ctx
	_ = project
	_ = service
	_ = lines
	return "unhealthy tail\n", nil
}

func TestServiceLogTailFromProvider(t *testing.T) {
	got := serviceLogTail(context.Background(), &logStub{}, "proj", "llama-chat", 80)
	if got != "unhealthy tail" {
		t.Fatalf("%q", got)
	}
}

func TestParseNumericUser(t *testing.T) {
	uid, gid, ok := parseNumericUser("1000")
	if !ok || uid != "1000" || gid != "1000" {
		t.Fatalf("%s %s %v", uid, gid, ok)
	}
	uid, gid, ok = parseNumericUser("35002")
	if !ok || uid != "35002" || gid != "35002" {
		t.Fatalf("tika %s %s %v", uid, gid, ok)
	}
	if _, _, ok := parseNumericUser("0"); ok {
		t.Fatal("root should skip unshare")
	}
	if _, _, ok := parseNumericUser("keycloak"); ok {
		t.Fatal("name should skip unshare")
	}
}
