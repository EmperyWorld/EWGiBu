package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
)

// RefsFromDir binds each regular file in dir to a secret id equal to the file name.
// File contents are not read. Names starting with '.' and subdirectories are skipped.
func RefsFromDir(dir string) ([]Ref, []diagnostic.Diagnostic) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "secrets dir: "+err.Error())}
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if name == "" || strings.HasPrefix(name, ".") || e.IsDir() {
			continue
		}
		if strings.ContainsAny(name, `/\`) {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid, fmt.Sprintf("secret file name %q is not a single path segment", name))}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Ref, 0, len(names))
	for _, name := range names {
		out = append(out, Ref{ID: name, Path: filepath.Join(dir, name)})
	}
	return out, nil
}

// Expand merges a secrets directory with explicit id=path pairs.
// An explicit pair replaces the same id from the directory.
func Expand(dir string, pairs []string) ([]string, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var fromDir []Ref
	if dir != "" {
		var ddiags []diagnostic.Diagnostic
		fromDir, ddiags = RefsFromDir(dir)
		diags = append(diags, ddiags...)
	}
	parsed, pdiags := ParsePairs(pairs)
	diags = append(diags, pdiags...)
	if diagnostic.HasError(diags) {
		return nil, diags
	}
	override := map[string]bool{}
	for _, r := range parsed {
		override[r.ID] = true
	}
	out := make([]string, 0, len(fromDir)+len(pairs))
	for _, r := range fromDir {
		if override[r.ID] {
			continue
		}
		out = append(out, r.ID+"="+r.Path)
	}
	out = append(out, pairs...)
	return out, nil
}
