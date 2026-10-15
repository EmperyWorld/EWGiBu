package workspace

import (
	"path/filepath"
)

// RootForManifest is the workspace root for a manifest path.
// A draft at <workspace>/imports/<file> uses <workspace>, the same root as
// import propose --workspace. A manifest in any other directory uses that
// directory, so iacpack.yaml at the project root stays the root.
func RootForManifest(filePath string) (string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) == "imports" {
		parent := filepath.Dir(dir)
		if parent != "" && parent != dir {
			return parent, nil
		}
	}
	return dir, nil
}
