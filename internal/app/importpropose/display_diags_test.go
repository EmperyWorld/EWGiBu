package importpropose

import (
	"testing"

	"iacpack/internal/diagnostic"
)

func TestDiagnosticsForAPIDedupesWarningRefusals(t *testing.T) {
	diags := []diagnostic.Diagnostic{
		diagnostic.Warning(diagnostic.CodeImportUnsupportedField, "service tika: \"logging\" omitted"),
		diagnostic.Info(diagnostic.CodeImportInterpolation, "mapped to parameter"),
	}
	refusals := []Refusal{{
		Code: diagnostic.CodeImportUnsupportedField, Message: "service tika: \"logging\" omitted",
	}}
	out := DiagnosticsForAPI(diags, refusals)
	if len(out) != 1 || out[0].Code != diagnostic.CodeImportInterpolation {
		t.Fatalf("got %#v", out)
	}
}
