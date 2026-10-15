package artifact_test

import (
	"testing"

	"iacpack/internal/artifact"
)

func TestCollisionAndContains(t *testing.T) {
	tr := artifact.New()
	if err := tr.AddFile("a/b.txt", artifact.ModeFile, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tr.AddFile("a/b.txt", artifact.ModeFile, []byte("y")); err == nil {
		t.Fatal("expected collision")
	}
	if err := tr.AddFile("a/b.txt/c", artifact.ModeFile, []byte("z")); err == nil {
		t.Fatal("expected nest collision")
	}
	if p, ok := tr.ContainsBytes([]byte("x")); !ok || p != "a/b.txt" {
		t.Fatalf("contains=%v %q", ok, p)
	}
}
