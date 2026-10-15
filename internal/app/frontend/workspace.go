package frontend

import (
	"path"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/inputs"
)

// outsideRoots lists workspace directories referenced by a pnpm workspace file
// or lockfile that sit outside context. Paths that escape the workspace or do
// not exist are errors. The returned paths are workspace-relative.
func outsideRoots(ws inputs.FS, id, ctx string) ([]string, []diagnostic.Diagnostic) {
	var tokens []string
	if raw, err := ws.ReadFile(path.Join(ctx, "pnpm-workspace.yaml")); err == nil {
		tokens = append(tokens, workspacePatterns(string(raw))...)
	}
	if raw, err := ws.ReadFile(path.Join(ctx, "pnpm-lock.yaml")); err == nil {
		tokens = append(tokens, lockfileOutside(string(raw))...)
	}
	seen := map[string]string{}
	var missing []diagnostic.Diagnostic
	for _, token := range tokens {
		rels, errMsg := resolveWorkspaceToken(ws, ctx, token)
		if errMsg != "" {
			missing = append(missing, diagnostic.Error(diagnostic.CodeBuildFrontend,
				"package build "+id+": "+errMsg))
			continue
		}
		for _, rel := range rels {
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = token
		}
	}
	if len(missing) > 0 {
		return nil, missing
	}
	roots := make([]string, 0, len(seen))
	for rel := range seen {
		roots = append(roots, rel)
	}
	sort.Strings(roots)
	return dropNested(roots), nil
}

func workspacePatterns(text string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if !in {
			if trim == "packages:" || strings.HasPrefix(trim, "packages:") {
				in = true
			}
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trim, "-") {
			break
		}
		if !strings.HasPrefix(trim, "-") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(trim, "-"))
		item = strings.Trim(item, `"'`)
		if item == "" || item == "." || item == "./" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func lockfileOutside(text string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		trim := strings.TrimSpace(line)
		if indent == 0 && strings.HasPrefix(trim, "importers:") {
			in = true
			continue
		}
		if in && indent == 0 {
			in = false
		}
		if in && indent == 2 {
			key := trim
			if i := strings.Index(key, ":"); i >= 0 {
				key = key[:i]
			}
			key = strings.Trim(key, `"'`)
			if key != "" && key != "." && key != "./" {
				out = append(out, key)
			}
		}
		if i := strings.Index(trim, "link:"); i >= 0 {
			link := strings.Trim(trim[i+len("link:"):], `"'`)
			if link != "" && link != "." {
				out = append(out, link)
			}
		}
	}
	return out
}

func resolveWorkspaceToken(ws inputs.FS, ctx, token string) ([]string, string) {
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, "\\") || strings.ContainsRune(token, 0) {
		return nil, "importer " + quote(token) + " is not a workspace-relative path"
	}
	if path.IsAbs(token) || strings.HasPrefix(token, "/") {
		return nil, "importer " + quote(token) + " escapes the workspace"
	}
	base, glob := splitGlob(token)
	rel := path.Clean(path.Join(ctx, base))
	if rel == "." {
		rel = ""
	}
	if rel == "" || rel == "." {
		return nil, ""
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, "importer " + quote(token) + " escapes the workspace"
	}
	if glob == "" {
		if insideContext(ctx, rel) {
			return nil, ""
		}
		if _, err := ws.Lstat(rel); err != nil {
			return nil, "importer " + quote(token) + " was not found at " + rel
		}
		return []string{rel}, ""
	}
	if _, err := ws.Lstat(rel); err != nil {
		return nil, "importer " + quote(token) + " was not found at " + rel
	}
	entries, err := ws.ReadDir(rel)
	if err != nil {
		return nil, "importer " + quote(token) + " was not found at " + rel
	}
	var matched []string
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." || name == "node_modules" || name == ".git" || !e.IsDir() {
			continue
		}
		if !matchGlob(glob, name) {
			continue
		}
		child := path.Join(rel, name)
		if insideContext(ctx, child) {
			continue
		}
		matched = append(matched, child)
	}
	sort.Strings(matched)
	if len(matched) == 0 {
		return nil, "importer " + quote(token) + " matched no directories"
	}
	return matched, ""
}

func splitGlob(token string) (base, glob string) {
	i := strings.IndexAny(token, "*?")
	if i < 0 {
		return token, ""
	}
	base = path.Dir(token[:i])
	if base == "." {
		base = ""
	}
	glob = token[i:]
	if slash := strings.IndexByte(glob, '/'); slash >= 0 {
		// Only the last segment is a glob. "../../packages/*" has the star at the end.
		glob = glob[:slash]
	}
	return base, glob
}

func matchGlob(glob, name string) bool {
	if glob == "*" || glob == "**" {
		return true
	}
	return name == glob
}

func insideContext(ctx, rel string) bool {
	if ctx == "" || ctx == "." {
		return true
	}
	return rel == ctx || strings.HasPrefix(rel, ctx+"/")
}

func dropNested(roots []string) []string {
	var out []string
	for _, rel := range roots {
		nested := false
		for _, other := range roots {
			if other != rel && strings.HasPrefix(rel, other+"/") {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, rel)
		}
	}
	return out
}

func quote(s string) string {
	return `"` + s + `"`
}
