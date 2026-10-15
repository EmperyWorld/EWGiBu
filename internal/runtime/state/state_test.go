package state_test

import (
	"testing"

	"iacpack/internal/runtime/state"
)

func TestLockExclusive(t *testing.T) {
	home := t.TempDir()
	p := state.ForInstance(home, "demo", "default")
	l1, err := state.AcquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Release()
	_, err = state.AcquireLock(p)
	if err == nil {
		t.Fatal("expected lock conflict")
	}
}

func TestResolveHome(t *testing.T) {
	got := state.ResolveHome("/tmp/xdg", "/home/u")
	if got != "/tmp/xdg/iacpack" {
		t.Fatal(got)
	}
}
