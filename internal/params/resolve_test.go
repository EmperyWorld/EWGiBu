package params_test

import (
	"os"
	"strings"
	"testing"

	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/params"
)

func TestPrecedenceSetOverValuesOverDefault(t *testing.T) {
	proj := domain.Project{Spec: domain.Spec{
		Parameters: map[string]domain.Parameter{
			"mode": {
				Phase:   domain.ParameterPhaseBuild,
				Type:    domain.ParameterTypeString,
				Default: &domain.Scalar{Kind: domain.ScalarString, Str: "default"},
			},
		},
	}}
	got, diags := params.ResolveBuild(proj, params.Options{
		Set:    map[string]string{"mode": "from-set"},
		Values: map[string]any{"mode": "from-values"},
	})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	if got["mode"].Value.Str != "from-set" || got["mode"].Source != params.SourceCLISet {
		t.Fatalf("%+v", got["mode"])
	}
	got, _ = params.ResolveBuild(proj, params.Options{Values: map[string]any{"mode": "from-values"}})
	if got["mode"].Value.Str != "from-values" {
		t.Fatal(got["mode"])
	}
	got, _ = params.ResolveBuild(proj, params.Options{})
	if got["mode"].Value.Str != "default" {
		t.Fatal(got["mode"])
	}
}

func TestRejectSecretLikeValuesKey(t *testing.T) {
	proj := domain.Project{Spec: domain.Spec{
		Parameters: map[string]domain.Parameter{
			"mode": {Phase: domain.ParameterPhaseBuild, Type: domain.ParameterTypeString, Default: &domain.Scalar{Kind: domain.ScalarString, Str: "x"}},
		},
	}}
	_, diags := params.ResolveBuild(proj, params.Options{Values: map[string]any{"db_password": "nope"}})
	if len(diags) == 0 {
		t.Fatal("expected secret-like rejection")
	}
}

func TestNoAmbientEnv(t *testing.T) {
	_ = os.Setenv("IACPACK_MODE", "from-env")
	t.Cleanup(func() { _ = os.Unsetenv("IACPACK_MODE") })
	proj := domain.Project{Spec: domain.Spec{
		Parameters: map[string]domain.Parameter{
			"mode": {Phase: domain.ParameterPhaseBuild, Type: domain.ParameterTypeString, Default: &domain.Scalar{Kind: domain.ScalarString, Str: "default"}},
		},
	}}
	got, diags := params.ResolveBuild(proj, params.Options{})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	if got["mode"].Value.Str != "default" {
		t.Fatalf("ambient env influenced value: %+v", got["mode"])
	}
}

func TestEmptyStringRejectedForTypedParams(t *testing.T) {
	proj := domain.Project{Spec: domain.Spec{
		Parameters: map[string]domain.Parameter{
			"workers": {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeInteger},
			"debug":   {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeBoolean},
		},
	}}
	_, diags := params.ResolveDeploy(proj, params.Options{Values: map[string]any{
		"workers": "",
		"debug":   "",
	}})
	if len(diags) < 2 {
		t.Fatalf("expected typed empty errors, got %v", diags)
	}
}

func TestValuesAllowTokenCountsAndTTLs(t *testing.T) {
	proj := domain.Project{Spec: domain.Spec{Parameters: map[string]domain.Parameter{
		"llama-chat-image-max-tokens": {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeInteger},
		"jitsi-token-ttl-seconds":     {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeInteger},
		"api-token":                   {Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeString},
	}}}
	_, diags := params.ResolveDeploy(proj, params.Options{Values: map[string]any{
		"llama-chat-image-max-tokens": 512,
		"jitsi-token-ttl-seconds":     3600,
		"api-token":                   "not-a-number",
	}})
	var secret []string
	for _, d := range diags {
		if d.Code == params.CodeParamSecretLike {
			secret = append(secret, d.Message)
		}
	}
	if len(secret) != 1 || !strings.Contains(secret[0], "api-token") {
		t.Fatalf("secret-like=%v all=%v", secret, diags)
	}
}
