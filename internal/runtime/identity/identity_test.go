package identity_test

import (
	"strings"
	"testing"

	"iacpack/internal/runtime/identity"
)

func TestProviderSlugStable(t *testing.T) {
	a := identity.ProviderSlug("demo", "default")
	b := identity.ProviderSlug("demo", "default")
	if a != b {
		t.Fatal("unstable slug")
	}
	if !strings.HasPrefix(a, "iacp-") {
		t.Fatalf("prefix %s", a)
	}
	if identity.ProviderSlug("demo", "other") == a {
		t.Fatal("instance must affect slug")
	}
}

func TestInstanceGrammar(t *testing.T) {
	if err := identity.ValidateInstance("default"); err != nil {
		t.Fatal(err)
	}
	if err := identity.ValidateInstance("Bad"); err == nil {
		t.Fatal("expected reject")
	}
}
