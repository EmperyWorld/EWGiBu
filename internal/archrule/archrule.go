// Package archrule enforces dependency direction and forbids remote module imports.
package archrule

import (
	"fmt"
	"strings"
)

// Rule describes allowed and forbidden import prefixes for a package class.
type Rule struct {
	Name              string
	PackagePrefixes   []string
	ForbiddenPrefixes []string
	// ForbidRemoteModules rejects imports whose first path segment contains a
	// dot (module-proxy style paths such as host.example/...).
	ForbidRemoteModules bool
}

// DefaultRules encodes the hexagonal boundary for this repository.
func DefaultRules() []Rule {
	return []Rule{
		{
			Name: "domain",
			PackagePrefixes: []string{
				"iacpack/internal/domain/",
			},
			ForbiddenPrefixes: []string{
				"iacpack/internal/adapters/",
				"iacpack/internal/runtime/dockercompose",
				"os/exec",
				"net/http",
			},
		},
		{
			Name: "application",
			PackagePrefixes: []string{
				"iacpack/internal/app/",
			},
			ForbiddenPrefixes: []string{
				"iacpack/internal/adapters/",
				"os/exec",
			},
		},
		{
			Name: "manifest",
			PackagePrefixes: []string{
				"iacpack/internal/manifest/",
			},
			ForbiddenPrefixes: []string{
				"iacpack/internal/adapters/",
				"os/exec",
				"net/http",
			},
		},
		{
			Name: "commands",
			PackagePrefixes: []string{
				"iacpack/cmd/",
			},
			ForbidRemoteModules: true,
		},
		{
			// Covers all internal packages (domain/app/manifest/adapters/...) once.
			Name: "all-internal-no-remote",
			PackagePrefixes: []string{
				"iacpack/internal/",
			},
			ForbidRemoteModules: true,
		},
	}
}

// Violation is one forbidden import edge.
type Violation struct {
	Package string
	Import  string
	Rule    string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s imports %s (rule %s)", v.Package, v.Import, v.Rule)
}

// CheckImports evaluates import paths for pkgPath against rules that match it.
func CheckImports(pkgPath string, imports []string, rules []Rule) []Violation {
	var out []Violation
	for _, rule := range rules {
		if !matchesAnyPrefix(pkgPath, rule.PackagePrefixes) {
			continue
		}
		for _, imp := range imports {
			if matchesAnyPrefix(imp, rule.ForbiddenPrefixes) {
				out = append(out, Violation{
					Package: pkgPath,
					Import:  imp,
					Rule:    rule.Name,
				})
				continue
			}
			if rule.ForbidRemoteModules && isRemoteModulePath(imp) {
				out = append(out, Violation{
					Package: pkgPath,
					Import:  imp,
					Rule:    rule.Name + ":remote-module",
				})
			}
		}
	}
	return out
}

// isRemoteModulePath reports module paths that look like network hostnames.
// Standard library imports (fmt, net/http, encoding/json) have no dot in the
// first path segment and are allowed.
func isRemoteModulePath(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return strings.Contains(first, ".")
}

func matchesAnyPrefix(value string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}
