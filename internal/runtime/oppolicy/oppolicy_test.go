package oppolicy_test

import (
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/policy"
	"iacpack/internal/runtime/oppolicy"
)

func TestOperatorCannotWeaken(t *testing.T) {
	compile := policy.MustDefault()
	raw := []byte(`{
	  "apiVersion":"iacpack.policy/v1",
	  "kind":"OperatorPolicy",
	  "metadata":{"name":"bad"},
	  "spec":{"capabilities":{"externalNetworks":true}}
	}`)
	op, err := oppolicy.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dep := ir.Deployment{Networks: map[string]ir.Network{}}
	diags := oppolicy.Evaluate(op, compile, dep)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected weaken denial")
	}
}

func TestOperatorDeniesExternal(t *testing.T) {
	compile := policy.MustDefault()
	op := oppolicy.Default()
	dep := ir.Deployment{
		Networks: map[string]ir.Network{"n": {External: &ir.ExternalRef{Name: "x"}}},
	}
	diags := oppolicy.Evaluate(op, compile, dep)
	found := false
	for _, d := range diags {
		if d.Code == oppolicy.CodeOpPolicyDeny {
			found = true
		}
	}
	if !found {
		t.Fatalf("want deny, got %v", diags)
	}
}
