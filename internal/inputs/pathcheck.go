package inputs

import (
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateRelPath enforces WorkspacePath lexical rules plus P03 Unicode policy:
// combining marks (Unicode category M) are rejected so NFD forms cannot collide
// with NFC under stdlib-only constraints (TZ.md §10.2).
func ValidateRelPath(raw string) error {
	if raw == "" {
		return fmt.Errorf("path is empty")
	}
	if !utf8.ValidString(raw) {
		return fmt.Errorf("path is not valid UTF-8")
	}
	if strings.ContainsRune(raw, 0) {
		return fmt.Errorf("path contains NUL")
	}
	if strings.Contains(raw, "\\") {
		return fmt.Errorf("path contains backslash")
	}
	if strings.HasPrefix(raw, "/") {
		return fmt.Errorf("path must be relative")
	}
	if len(raw) >= 2 && raw[1] == ':' {
		return fmt.Errorf("path must not contain a drive prefix")
	}
	if strings.HasPrefix(raw, "//") {
		return fmt.Errorf("path must not be UNC")
	}
	for _, r := range raw {
		if unicode.Is(unicode.M, r) {
			return fmt.Errorf("path must not contain combining characters; use Unicode NFC")
		}
	}
	cleaned := path.Clean("/" + raw)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path escapes workspace")
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" {
			return fmt.Errorf("path contains empty segment")
		}
		if seg == ".." {
			return fmt.Errorf("path must not contain parent segments")
		}
		if seg == "." {
			return fmt.Errorf("path must not contain current-directory segments")
		}
	}
	return nil
}

// CollisionKey returns a case-folded key used to detect case-folding collisions.
func CollisionKey(p string) string {
	return strings.ToLower(p)
}
