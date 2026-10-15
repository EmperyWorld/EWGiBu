package secrets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/secrets"
)

func TestSecretValidateNoContentInDiags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	canary := "SUPER_SECRET_CANARY_VALUE_9f3a"
	if err := os.WriteFile(path, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	declared := map[string]ir.Secret{"db": {Required: true, Delivery: "file"}}
	diags := secrets.Validate(declared, []secrets.Ref{{ID: "db", Path: path}})
	if diagnostic.HasError(diags) {
		t.Fatalf("unexpected %v", diags)
	}
	// missing required
	diags = secrets.Validate(declared, nil)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected missing")
	}
	blob := ""
	for _, d := range diags {
		blob += d.Message + d.Hint
	}
	if strings.Contains(blob, canary) {
		t.Fatal("canary leaked into diagnostics")
	}
}
