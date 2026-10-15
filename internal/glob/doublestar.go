// Package glob implements documented doublestar matching for workspace includes.
//
// Semantics (TZ.md §10.2):
//   - Patterns and paths use '/' separators only (never '\').
//   - '*' matches any sequence within a single path segment (not across '/').
//   - '?' matches a single character within a segment (not '/').
//   - '**' matches zero or more full path segments, including the empty match.
//   - A pattern that is exactly '**' matches every relative file path.
//   - Matching is against the relative path from the resource/build source root.
//   - Character classes and brace expansion are not supported in v1.
package glob

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Match reports whether name (slash-separated relative path) matches pattern.
func Match(pattern, name string) (bool, error) {
	if err := Validate(pattern); err != nil {
		return false, err
	}
	if strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
		return false, fmt.Errorf("glob: invalid name")
	}
	return matchSegments(splitSegs(pattern), splitSegs(name)), nil
}

// Validate checks a pattern without matching a name.
func Validate(pattern string) error {
	return validatePattern(pattern)
}

func validatePattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("glob: empty pattern")
	}
	if strings.Contains(pattern, "\\") || strings.ContainsRune(pattern, 0) {
		return fmt.Errorf("glob: pattern must use '/' separators only")
	}
	if strings.Contains(pattern, "[") || strings.Contains(pattern, "{") {
		return fmt.Errorf("glob: character classes and braces are not supported")
	}
	for _, seg := range splitSegs(pattern) {
		if seg == "" {
			return fmt.Errorf("glob: empty pattern segment")
		}
		if strings.Contains(seg, "**") && seg != "**" {
			return fmt.Errorf("glob: '**' must be a whole segment")
		}
	}
	return nil
}

func splitSegs(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			pat = pat[1:]
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if !matchSegment(pat[0], name[0]) {
			return false
		}
		pat = pat[1:]
		name = name[1:]
	}
	return len(name) == 0
}

func matchSegment(pat, name string) bool {
	pi, ni := 0, 0
	for pi < len(pat) {
		pr, psz := utf8.DecodeRuneInString(pat[pi:])
		if pr == '*' {
			pi += psz
			if pi == len(pat) {
				return !strings.Contains(name[ni:], "/")
			}
			for i := ni; i <= len(name); i++ {
				if matchSegment(pat[pi:], name[i:]) {
					return true
				}
				if i < len(name) {
					_, wsz := utf8.DecodeRuneInString(name[i:])
					i += wsz - 1
				}
			}
			return false
		}
		if ni >= len(name) {
			return false
		}
		nr, nsz := utf8.DecodeRuneInString(name[ni:])
		if pr == '?' {
			if nr == '/' {
				return false
			}
			pi += psz
			ni += nsz
			continue
		}
		if pr != nr {
			return false
		}
		pi += psz
		ni += nsz
	}
	return ni == len(name)
}

// Reaches reports whether pattern can match some file inside dir.
// dir is slash-relative to the include root. An empty dir is the root itself.
func Reaches(pattern, dir string) (bool, error) {
	if err := Validate(pattern); err != nil {
		return false, err
	}
	if dir == "" || dir == "." {
		return true, nil
	}
	if strings.Contains(dir, "\\") || strings.ContainsRune(dir, 0) {
		return false, fmt.Errorf("glob: invalid name")
	}
	return prefixCanReach(splitSegs(pattern), splitSegs(dir)), nil
}

func prefixCanReach(pat, dir []string) bool {
	pi, di := 0, 0
	for di < len(dir) {
		if pi >= len(pat) {
			return false
		}
		if pat[pi] == "**" {
			if pi == len(pat)-1 {
				return true
			}
			for k := di; k <= len(dir); k++ {
				if prefixCanReach(pat[pi+1:], dir[k:]) {
					return true
				}
			}
			return false
		}
		if !matchSegment(pat[pi], dir[di]) {
			return false
		}
		pi++
		di++
	}
	return pi < len(pat)
}
