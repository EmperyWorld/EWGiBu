// Package artifact implements an in-memory virtual artifact tree for deterministic builds.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// FileMode is a normalized octal mode string (e.g. "0644", "0755").
type FileMode string

const (
	ModeFile FileMode = "0644"
	ModeExec FileMode = "0755"
	ModeDir  FileMode = "0755"
)

// Entry is one file in the virtual tree (directories are implied by paths).
type Entry struct {
	Path    string // slash-separated BundlePath
	Mode    FileMode
	Content []byte
	SHA256  string
}

// Tree is a collision-checked virtual artifact tree.
type Tree struct {
	entries map[string]Entry
}

// New returns an empty tree.
func New() *Tree {
	return &Tree{entries: map[string]Entry{}}
}

// AddFile adds a regular file. Path must be a relative BundlePath (no .., abs, etc.).
func (t *Tree) AddFile(path string, mode FileMode, content []byte) error {
	if err := validateBundlePath(path); err != nil {
		return err
	}
	if mode == "" {
		mode = ModeFile
	}
	if _, ok := t.entries[path]; ok {
		return fmt.Errorf("artifact path collision: %s", path)
	}
	// Reject if path is a prefix of an existing file or vice versa (dir/file clash).
	for existing := range t.entries {
		if strings.HasPrefix(existing, path+"/") {
			return fmt.Errorf("artifact path collision: %q is a directory prefix of %q", path, existing)
		}
		if strings.HasPrefix(path, existing+"/") {
			return fmt.Errorf("artifact path collision: %q nests under file %q", path, existing)
		}
	}
	sum := sha256.Sum256(content)
	// Keep a non-nil slice so a 0-byte file is distinct from "content not read".
	cp := append([]byte{}, content...)
	t.entries[path] = Entry{
		Path:    path,
		Mode:    mode,
		Content: cp,
		SHA256:  hex.EncodeToString(sum[:]),
	}
	return nil
}

// Get returns an entry if present.
func (t *Tree) Get(path string) (Entry, bool) {
	e, ok := t.entries[path]
	return e, ok
}

// Sorted returns entries ordered by UTF-8 path bytes.
func (t *Tree) Sorted() []Entry {
	paths := make([]string, 0, len(t.entries))
	for p := range t.entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]Entry, 0, len(paths))
	for _, p := range paths {
		out = append(out, t.entries[p])
	}
	return out
}

// Paths returns sorted paths.
func (t *Tree) Paths() []string {
	ents := t.Sorted()
	out := make([]string, len(ents))
	for i, e := range ents {
		out[i] = e.Path
	}
	return out
}

// ContainsBytes reports whether any file content contains needle (secret canary tests).
func (t *Tree) ContainsBytes(needle []byte) (string, bool) {
	if len(needle) == 0 {
		return "", false
	}
	for _, e := range t.Sorted() {
		if bytesContains(e.Content, needle) {
			return e.Path, true
		}
	}
	return "", false
}

func bytesContains(hay, needle []byte) bool {
	if len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := 0; j < len(needle); j++ {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func validateBundlePath(p string) error {
	if p == "" {
		return fmt.Errorf("empty artifact path")
	}
	if strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return fmt.Errorf("invalid artifact path %q", p)
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("artifact path must be relative: %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid artifact path segment in %q", p)
		}
	}
	if len(p) > 255 {
		return fmt.Errorf("artifact path exceeds 255 bytes: %q", p)
	}
	return nil
}
