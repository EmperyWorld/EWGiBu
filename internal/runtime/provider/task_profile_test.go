package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"iacpack/internal/runtime/process"
)

func TestRunTaskPassesIacpackProfile(t *testing.T) {
	rec := &process.RecordingRunner{}
	h := HostApply{Host: Host{Runner: rec, Engine: "podman", Socket: "/tmp/test.sock"}}
	if err := h.RunTask(context.Background(), "proj", "/tmp/compose.yaml", "eliteew-sql-migrate", time.Second); err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) != 1 {
		t.Fatalf("calls=%d", len(rec.Calls))
	}
	got := strings.Join(rec.Calls[0].Spec.Args, " ")
	want := "compose --profile iacpack-task -p proj -f /tmp/compose.yaml run -T --rm --no-deps eliteew-sql-migrate"
	if got != want {
		t.Fatalf("argv=%q", got)
	}
}
