package workspaceingest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
)

// DirEntry is one child of a workspace directory (names only, no file bytes).
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"` // file | dir
}

// ListDir lists one workspace directory. dir empty means the workspace root.
// Symlinks are skipped. Hidden VCS and secret filenames are omitted.
func ListDir(workspace, dir string) (string, []DirEntry, []diagnostic.Diagnostic) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "workspace is required")}
	}
	rel := filepath.ToSlash(strings.TrimSpace(dir))
	rel = strings.Trim(rel, "/")
	if rel == "." {
		rel = ""
	}
	if rel != "" {
		clean, err := cleanWorkspaceRel(rel, DefaultLimits().MaxPathBytes)
		if err != nil {
			return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
		}
		rel = clean
	}
	abs := workspace
	if rel != "" {
		abs = filepath.Join(workspace, filepath.FromSlash(rel))
	}
	if !underRoot(workspace, abs) {
		return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "path escapes workspace")}
	}
	f, err := os.Open(abs)
	if err != nil {
		return rel, nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
	}
	defer f.Close()
	infos, err := f.Readdir(501)
	if err != nil && err != io.EOF {
		return rel, nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
	}
	if len(infos) > 500 {
		return rel, nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "directory has more than 500 entries; narrow the path")}
	}
	var out []DirEntry
	for _, info := range infos {
		name := info.Name()
		if skipWorkspaceName(name) {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		} else if !info.Mode().IsRegular() {
			continue
		}
		out = append(out, DirEntry{Name: name, Path: joinRel(rel, name), Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "dir"
		}
		return out[i].Name < out[j].Name
	})
	return rel, out, nil
}

func skipWorkspaceName(name string) bool {
	low := strings.ToLower(name)
	if name == ".git" || name == ".iacpack-out" {
		return true
	}
	if low == ".env" || strings.HasPrefix(low, ".env.") {
		return true
	}
	return false
}

func joinRel(dir, name string) string {
	if dir == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", dir, name)
}
