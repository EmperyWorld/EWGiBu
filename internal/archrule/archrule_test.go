package archrule_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"iacpack/internal/archrule"
)

func TestForbiddenAdapterImportFixtureFails(t *testing.T) {
	rules := archrule.DefaultRules()
	violations := archrule.CheckImports(
		"iacpack/internal/domain/ir",
		[]string{
			"fmt",
			"iacpack/internal/adapters/cli",
		},
		rules,
	)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %#v", violations)
	}
	if violations[0].Import != "iacpack/internal/adapters/cli" {
		t.Fatalf("unexpected violation: %v", violations[0])
	}
}

func TestRemoteModuleImportFixtureFails(t *testing.T) {
	violations := archrule.CheckImports(
		"iacpack/internal/app/versioninfo",
		[]string{"example.com/foreign/lib"},
		archrule.DefaultRules(),
	)
	if len(violations) != 1 {
		t.Fatalf("expected remote-module violation, got %#v", violations)
	}
}

func TestAllowedDomainImportsPass(t *testing.T) {
	violations := archrule.CheckImports(
		"iacpack/internal/domain/ir",
		[]string{"fmt", "strings", "iacpack/internal/domain/plan"},
		archrule.DefaultRules(),
	)
	if len(violations) != 0 {
		t.Fatalf("unexpected violations: %#v", violations)
	}
}

func TestRepositoryPackagesRespectArchitecture(t *testing.T) {
	root := repoRoot(t)
	rules := archrule.DefaultRules()
	var all []archrule.Violation

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkgPath := "iacpack/" + filepath.ToSlash(rel)
		imports, err := parseImports(path)
		if err != nil {
			return err
		}
		all = append(all, archrule.CheckImports(pkgPath, imports, rules)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Also scan cmd/
	err = filepath.WalkDir(filepath.Join(root, "cmd"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkgPath := "iacpack/" + filepath.ToSlash(rel)
		imports, err := parseImports(path)
		if err != nil {
			return err
		}
		all = append(all, archrule.CheckImports(pkgPath, imports, rules)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(all) != 0 {
		t.Fatalf("architecture violations: %v", all)
	}
}

func parseImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out, nil
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
