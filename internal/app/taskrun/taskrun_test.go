package taskrun_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/app/taskrun"
	"iacpack/internal/runtime/provider"
	"iacpack/internal/runtime/state"
)

func TestTaskRunRequiresManualRerun(t *testing.T) {
	home := t.TempDir()
	paths := state.ForInstance(home, "demo", "default")
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := state.Document{
		FormatVersion: state.FormatVersion,
		Project:       "demo",
		Instance:      "default",
		ProviderSlug:  "iacp-demo-default-deadbeef",
		Active:        &state.ActiveBundle{Digest: "abc", SemanticHash: "def", Path: filepath.Join(home, "missing.bundle")},
	}
	raw, _ := json.Marshal(doc)
	if err := os.WriteFile(paths.State, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &provider.RecordingApply{}
	res := taskrun.Run(context.Background(), taskrun.Options{
		Project: "demo", Task: "seed", StateHome: home, Yes: true, Provider: rec,
	})
	if res.OK {
		t.Fatal("expected failure without readable bundle")
	}
	if len(rec.Calls) != 0 {
		t.Fatalf("provider must not run: %v", rec.Calls)
	}
}

func TestTaskRunRejectsWithoutYes(t *testing.T) {
	rec := &provider.RecordingApply{}
	res := taskrun.Run(context.Background(), taskrun.Options{
		Project: "demo", Task: "seed", Yes: false, Provider: rec, StateHome: t.TempDir(),
	})
	if res.OK {
		t.Fatal("expected fail")
	}
}
