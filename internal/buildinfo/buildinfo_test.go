package buildinfo_test

import (
	"testing"

	"iacpack/internal/buildinfo"
)

func TestDevelopmentDefaults(t *testing.T) {
	if buildinfo.Product != "iacpack" {
		t.Fatalf("Product=%q", buildinfo.Product)
	}
	if buildinfo.Version != "1.1.0" {
		t.Fatalf("Version=%q", buildinfo.Version)
	}
	if buildinfo.Commit != "unknown" {
		t.Fatalf("Commit=%q", buildinfo.Commit)
	}
	if buildinfo.DirtyBool() {
		t.Fatal("DirtyBool unexpectedly true")
	}
}
