// Package discover implements deterministic init/inspect --discover suggestions.
package discover

import (
	"path"
	"sort"
	"strings"

	"iacpack/internal/inputs"
)

// Suggestion is a reviewable discovery hit (never applied silently).
type Suggestion struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// MaxDepth is the maximum directory depth from the workspace root (TZ.md §10).
const MaxDepth = 3

func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if strings.Contains(lower, "secret") || strings.Contains(lower, "credential") {
		return true
	}
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
		return true
	}
	if lower == "id_rsa" || lower == "id_ed25519" {
		return true
	}
	return false
}

func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

func isOutputRoot(name string) bool {
	switch name {
	case "dist", "build", "out", "output", "bin", ".iacpack":
		return true
	default:
		return false
	}
}

// ClassifyKind returns the suggestion kind for a base filename, or "".
// Patterns follow TZ.md §10 / ADR-017 (deterministic, reviewable only).
func ClassifyKind(name string) string {
	switch name {
	case "Dockerfile":
		return "dockerfile"
	case "compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml":
		return "compose"
	case "Caddyfile":
		return "caddy"
	case "nginx.conf":
		return "nginx"
	}
	if strings.HasPrefix(name, "Dockerfile.") {
		return "dockerfile"
	}
	lower := strings.ToLower(name)
	if isComposePattern(lower) {
		return "compose"
	}
	return ""
}

func isComposePattern(lower string) bool {
	if lower == "compose.yaml" || lower == "compose.yml" ||
		lower == "docker-compose.yml" || lower == "docker-compose.yaml" {
		return true
	}
	if strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") {
		if strings.HasPrefix(lower, "compose.") {
			return true
		}
		if strings.HasPrefix(lower, "docker-compose.") {
			return true
		}
		if strings.HasPrefix(lower, "stack") {
			return true
		}
	}
	return false
}

// Run scans a workspace inputs.FS for well-known files up to MaxDepth.
func Run(ws inputs.FS) ([]Suggestion, error) {
	var out []Suggestion
	var walk func(rel string, depth int) error
	walk = func(rel string, depth int) error {
		entries, err := ws.ReadDir(rel)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(entries))
		byName := map[string]inputs.DirEntry{}
		for _, e := range entries {
			names = append(names, e.Name())
			byName[e.Name()] = e
		}
		sort.Strings(names)
		for _, name := range names {
			e := byName[name]
			if isHidden(name) || isSensitive(name) {
				continue
			}
			child := name
			if rel != "" && rel != "." {
				child = path.Join(rel, name)
			}
			child = strings.ReplaceAll(child, "\\", "/")
			if e.IsDir() {
				if isOutputRoot(name) || depth >= MaxDepth {
					continue
				}
				if err := walk(child, depth+1); err != nil {
					return err
				}
				continue
			}
			kind := ClassifyKind(name)
			if kind == "" {
				continue
			}
			out = append(out, Suggestion{Path: child, Kind: kind})
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
