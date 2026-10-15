// Package path defines typed path domains with lexical validation.
// Filesystem containment, glob expansion, and special-file rejection live in
// package inputs (P03).
package path

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// WorkspacePath is a relative path from the manifest root.
type WorkspacePath string

// BundlePath is a relative controlled path inside a bundle.
type BundlePath string

// ContainerPath is an absolute POSIX path inside a container.
type ContainerPath string

// ManagedHostPath is a path under the runtime state root.
type ManagedHostPath string

// ExternalHostPath is an operator capability forbidden by default policy.
type ExternalHostPath string

// ParseWorkspacePath validates lexical WorkspacePath rules (TZ.md §10.2).
func ParseWorkspacePath(raw string) (WorkspacePath, error) {
	if err := rejectEmpty(raw); err != nil {
		return "", err
	}
	if err := rejectAbsoluteOrTraversal(raw); err != nil {
		return "", err
	}
	return WorkspacePath(raw), nil
}

// ParseBundlePath validates lexical BundlePath rules (relative, no traversal).
func ParseBundlePath(raw string) (BundlePath, error) {
	if err := rejectEmpty(raw); err != nil {
		return "", err
	}
	if err := rejectAbsoluteOrTraversal(raw); err != nil {
		return "", err
	}
	return BundlePath(raw), nil
}

// ParseContainerPath validates an absolute POSIX container path.
func ParseContainerPath(raw string) (ContainerPath, error) {
	if err := rejectEmpty(raw); err != nil {
		return "", err
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("path contains NUL")
	}
	if strings.Contains(raw, "\\") {
		return "", fmt.Errorf("path contains backslash")
	}
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("container path must be absolute")
	}
	if strings.Contains(raw, "//") {
		return "", fmt.Errorf("container path must not contain empty segments")
	}
	parts := strings.Split(raw, "/")
	for _, p := range parts[1:] {
		if p == ".." {
			return "", fmt.Errorf("container path must not contain parent segments")
		}
	}
	return ContainerPath(raw), nil
}

// ParseManagedHostPath validates lexical managed host path form (absolute POSIX).
func ParseManagedHostPath(raw string) (ManagedHostPath, error) {
	if err := rejectEmpty(raw); err != nil {
		return "", err
	}
	if strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("invalid managed host path")
	}
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("managed host path must be absolute")
	}
	return ManagedHostPath(raw), nil
}

// ParseExternalHostPath validates lexical external host path form.
func ParseExternalHostPath(raw string) (ExternalHostPath, error) {
	if err := rejectEmpty(raw); err != nil {
		return "", err
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("path contains NUL")
	}
	return ExternalHostPath(raw), nil
}

func rejectEmpty(raw string) error {
	if raw == "" {
		return fmt.Errorf("path is empty")
	}
	if !utf8.ValidString(raw) {
		return fmt.Errorf("path is not valid UTF-8")
	}
	if strings.ContainsRune(raw, 0) {
		return fmt.Errorf("path contains NUL")
	}
	return nil
}

func rejectAbsoluteOrTraversal(raw string) error {
	if strings.Contains(raw, "\\") {
		return fmt.Errorf("path contains backslash")
	}
	if strings.HasPrefix(raw, "/") {
		return fmt.Errorf("path must be relative")
	}
	// Drive / UNC style prefixes.
	if len(raw) >= 2 && raw[1] == ':' {
		return fmt.Errorf("path must not contain a drive prefix")
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(strings.ToLower(raw), "\\\\") {
		return fmt.Errorf("path must not be UNC")
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" {
			return fmt.Errorf("path contains empty segment")
		}
		if seg == ".." {
			return fmt.Errorf("path must not contain parent segments")
		}
	}
	return nil
}

// String returns the underlying path text.
func (p WorkspacePath) String() string   { return string(p) }
func (p BundlePath) String() string      { return string(p) }
func (p ContainerPath) String() string   { return string(p) }
func (p ManagedHostPath) String() string { return string(p) }
func (p ExternalHostPath) String() string {
	return string(p)
}
