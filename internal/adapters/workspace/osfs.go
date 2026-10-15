// Package workspace adapts the OS filesystem as a rooted inputs.FS.
package workspace

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/inputs"
)

// OS is a workspace rooted at Root (absolute).
type OS struct {
	Root string
}

// New returns an OS workspace for an absolute root directory.
func New(root string) (OS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return OS{}, err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return OS{}, err
	}
	if !fi.IsDir() {
		return OS{}, fmt.Errorf("workspace root is not a directory")
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return OS{}, fmt.Errorf("workspace root must not be a symlink")
	}
	return OS{Root: abs}, nil
}

type fileInfo struct {
	name  string
	size  int64
	mode  fs.FileMode
	isDir bool
	nlink uint64
}

func (f fileInfo) Name() string      { return f.name }
func (f fileInfo) Size() int64       { return f.size }
func (f fileInfo) Mode() fs.FileMode { return f.mode }
func (f fileInfo) IsDir() bool       { return f.isDir }
func (f fileInfo) Nlink() uint64     { return f.nlink }

type dirEntry struct {
	name  string
	isDir bool
	typ   fs.FileMode
	info  fileInfo
}

func (d dirEntry) Name() string                   { return d.name }
func (d dirEntry) IsDir() bool                    { return d.isDir }
func (d dirEntry) Type() fs.FileMode              { return d.typ }
func (d dirEntry) Info() (inputs.FileInfo, error) { return d.info, nil }

func (w OS) resolve(rel string) (string, error) {
	if rel == "" || rel == "." {
		return w.Root, nil
	}
	if err := inputs.ValidateRelPath(rel); err != nil {
		return "", err
	}
	// Convert slash path to OS path under root without cleaning away containment.
	parts := strings.Split(rel, "/")
	full := w.Root
	for _, p := range parts {
		full = filepath.Join(full, p)
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	rootWith := w.Root
	if !strings.HasSuffix(rootWith, sep) {
		rootWith += sep
	}
	if abs != w.Root && !strings.HasPrefix(abs+sep, rootWith) && !strings.HasPrefix(abs, rootWith) {
		return "", fmt.Errorf("path escapes workspace")
	}
	return abs, nil
}

func (w OS) Lstat(rel string) (inputs.FileInfo, error) {
	abs, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	return toInfo(fi), nil
}

func (w OS) ReadDir(rel string) ([]inputs.DirEntry, error) {
	abs, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]inputs.DirEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		fi := toInfo(info)
		out = append(out, dirEntry{
			name:  e.Name(),
			isDir: e.IsDir(),
			typ:   e.Type(),
			info:  fi.(fileInfo),
		})
	}
	return out, nil
}

func (w OS) Open(rel string) (io.ReadCloser, error) {
	abs, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	// Open without following symlinks: O_NOFOLLOW where available.
	return openNoFollow(abs)
}

// FollowSymlink resolves rel to a workspace-relative path. Symlink chains are
// followed only while every target stays inside the workspace root.
func (w OS) FollowSymlink(rel string) (string, error) {
	cur := rel
	if cur == "" {
		cur = "."
	}
	seen := map[string]struct{}{}
	for step := 0; step < 8; step++ {
		if _, ok := seen[cur]; ok {
			return "", fmt.Errorf("symlink cycle")
		}
		seen[cur] = struct{}{}
		abs, err := w.resolve(cur)
		if err != nil {
			return "", err
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			if cur == "." {
				return ".", nil
			}
			return cur, nil
		}
		raw, err := os.Readlink(abs)
		if err != nil {
			return "", err
		}
		nextAbs := raw
		if !filepath.IsAbs(raw) {
			nextAbs = filepath.Join(filepath.Dir(abs), raw)
		}
		nextAbs = filepath.Clean(nextAbs)
		next, err := w.relInside(nextAbs)
		if err != nil {
			return "", err
		}
		cur = next
	}
	return "", fmt.Errorf("symlink chain too long")
}

func (w OS) relInside(abs string) (string, error) {
	rel, err := filepath.Rel(w.Root, filepath.Clean(abs))
	if err != nil {
		return "", fmt.Errorf("symlink escapes workspace")
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("symlink escapes workspace")
	}
	if rel == "." {
		return ".", nil
	}
	if err := inputs.ValidateRelPath(rel); err != nil {
		return "", fmt.Errorf("symlink escapes workspace")
	}
	return rel, nil
}

func (w OS) ReadFile(rel string) ([]byte, error) {
	f, err := w.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func toInfo(fi os.FileInfo) inputs.FileInfo {
	nlink := uint64(1)
	if n, ok := nlinkOf(fi); ok {
		nlink = n
	}
	return fileInfo{
		name:  fi.Name(),
		size:  fi.Size(),
		mode:  fi.Mode(),
		isDir: fi.IsDir(),
		nlink: nlink,
	}
}

var _ inputs.FS = OS{}
