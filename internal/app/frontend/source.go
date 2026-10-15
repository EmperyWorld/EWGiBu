package frontend

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/inputs"
)

var (
	fromSpecRe    = regexp.MustCompile(`\bfrom\s+['"](\.{1,2}/[^'"\n]+)['"]`)
	sideImportRe  = regexp.MustCompile(`(?:^|[;\n])\s*import\s+['"](\.{1,2}/[^'"\n]+)['"]`)
	dynImportRe   = regexp.MustCompile(`\bimport\s*\(\s*['"](\.{1,2}/[^'"\n]+)['"]`)
	requireSpecRe = regexp.MustCompile(`\brequire\s*\(\s*['"](\.{1,2}/[^'"\n]+)['"]`)
)

// sourceOutside finds relative imports that leave context and name an existing
// workspace file or directory. Missing specifiers are ignored. A specifier that
// leaves the workspace is an error.
func sourceOutside(ws inputs.FS, id, ctx string) (files, dirs []string, diags []diagnostic.Diagnostic) {
	seenFile := map[string]struct{}{}
	seenDir := map[string]struct{}{}
	var pending []string
	_ = walkContextSources(ws, ctx, func(rel string) {
		pending = append(pending, rel)
	})
	scanned := map[string]struct{}{}
	for len(pending) > 0 && len(scanned) < 20000 {
		rel := pending[0]
		pending = pending[1:]
		if _, ok := scanned[rel]; ok {
			continue
		}
		scanned[rel] = struct{}{}
		if !isImportSource(rel) {
			continue
		}
		body, err := ws.ReadFile(rel)
		if err != nil || len(body) > 2<<20 {
			continue
		}
		for _, spec := range relativeSpecs(string(body)) {
			target, isDir, ok, errMsg := resolveImport(ws, ctx, rel, spec)
			if errMsg != "" {
				diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
					"package build "+id+": import "+quote(spec)+" "+errMsg))
				continue
			}
			if !ok {
				continue
			}
			if isDir {
				if _, dup := seenDir[target]; dup {
					continue
				}
				seenDir[target] = struct{}{}
				dirs = append(dirs, target)
				continue
			}
			if _, dup := seenFile[target]; dup {
				continue
			}
			seenFile[target] = struct{}{}
			files = append(files, target)
			if isImportSource(target) {
				pending = append(pending, target)
			}
		}
	}
	sort.Strings(files)
	sort.Strings(dirs)
	return files, dirs, diags
}

func walkContextSources(ws inputs.FS, ctx string, fn func(rel string)) error {
	root := ctx
	if root == "" {
		root = "."
	}
	var walk func(rel string) error
	walk = func(rel string) error {
		entries, err := ws.ReadDir(rel)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if name == "." || name == ".." || name == "node_modules" || name == ".git" {
				continue
			}
			child := name
			if rel != "." {
				child = path.Join(rel, name)
			}
			if e.IsDir() {
				if err := walk(child); err != nil {
					return err
				}
				continue
			}
			if isImportSource(child) {
				fn(child)
			}
		}
		return nil
	}
	return walk(root)
}

func isImportSource(rel string) bool {
	switch strings.ToLower(path.Ext(rel)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".vue", ".svelte", ".css", ".scss", ".sass", ".less":
		return true
	default:
		return false
	}
}

func relativeSpecs(text string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(spec string, idx int) {
		if !strings.Contains(spec, "..") || strings.ContainsAny(spec, "\\*?") {
			return
		}
		if _, ok := seen[spec]; ok {
			return
		}
		if !statementImports(text, idx) {
			return
		}
		seen[spec] = struct{}{}
		out = append(out, spec)
	}
	for _, re := range []*regexp.Regexp{fromSpecRe, sideImportRe, dynImportRe, requireSpecRe} {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(m) < 4 {
				continue
			}
			add(text[m[2]:m[3]], m[0])
		}
	}
	return out
}

// statementImports reports that the match sits in an import, export, or require
// statement. A quoted path in join() or a comment is not an import.
func statementImports(text string, idx int) bool {
	start := idx
	for start > 0 && text[start-1] != ';' {
		start--
		if idx-start > 2000 {
			break
		}
	}
	end := idx + 24
	if end > len(text) {
		end = len(text)
	}
	window := text[start:end]
	return strings.Contains(window, "import") || strings.Contains(window, "export") || strings.Contains(window, "require")
}

func resolveImport(ws inputs.FS, ctx, fromFile, spec string) (rel string, isDir, ok bool, errMsg string) {
	if path.IsAbs(spec) || strings.HasPrefix(spec, "/") || strings.Contains(spec, "\\") {
		return "", false, false, "escapes the workspace"
	}
	rel = path.Clean(path.Join(path.Dir(fromFile), spec))
	if rel == "" || rel == "." {
		return "", false, false, ""
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false, false, "escapes the workspace"
	}
	if insideContext(ctx, rel) {
		return "", false, false, ""
	}
	if info, err := ws.Lstat(rel); err == nil {
		return rel, info.IsDir(), true, ""
	}
	if path.Ext(spec) != "" {
		return "", false, false, ""
	}
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".json", ".css", ".vue"} {
		candidate := rel + ext
		if info, err := ws.Lstat(candidate); err == nil && !info.IsDir() {
			if insideContext(ctx, candidate) {
				return "", false, false, ""
			}
			return candidate, false, true, ""
		}
	}
	return "", false, false, ""
}
