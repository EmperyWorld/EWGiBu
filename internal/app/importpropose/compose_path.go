package importpropose

import (
	"fmt"
	stdpath "path"
	"path/filepath"
	"strings"

	"iacpack/internal/domain/path"
)

// resolveComposeWorkspacePath maps a Compose-relative path to a workspace-relative path.
// When the compose file sits at the workspace root, parent segments (../) cannot be
// resolved against the file directory; those paths are rebased to the workspace root
// (import-only heuristic for stacks copied from docker/stack layouts).
func resolveComposeWorkspacePath(composeFile, rel string) (resolved string, usedFallback bool, err error) {
	resolved, err = resolveComposeWorkspacePathStrict(composeFile, rel)
	if err == nil {
		return resolved, false, nil
	}
	if !isComposeParentEscape(err) {
		return "", false, err
	}
	fb, ok := rebaseParentRelativeToWorkspaceRoot(rel)
	if !ok {
		return "", false, err
	}
	return fb, true, nil
}

func resolveComposeWorkspacePathStrict(composeFile, rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		rel = "."
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("absolute path")
	}
	base := filepath.ToSlash(filepath.Dir(composeFile))
	if base == "." {
		base = ""
	}
	combined := rel
	if base != "" {
		combined = base + "/" + rel
	}
	cleaned := stdpath.Clean(combined)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("escapes workspace via parent segments")
	}
	return cleaned, nil
}

func isComposeParentEscape(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent segments")
}

func rebaseParentRelativeToWorkspaceRoot(rel string) (string, bool) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return "", false
	}
	parts := strings.Split(rel, "/")
	var out []string
	for _, p := range parts {
		switch p {
		case "", ".":
			continue
		case "..":
			continue
		default:
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "", false
	}
	cleaned := stdpath.Clean(strings.Join(out, "/"))
	if cleaned == "." || cleaned == "" || strings.Contains(cleaned, "..") {
		return "", false
	}
	if _, err := path.ParseWorkspacePath(cleaned); err != nil {
		return "", false
	}
	return cleaned, true
}
