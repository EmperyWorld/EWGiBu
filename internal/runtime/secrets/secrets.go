// Package secrets validates operator secret-file references without loading values into diagnostics.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
)

const (
	CodeSecretMissing = "SECRET.MISSING"
	CodeSecretInvalid = "SECRET.INVALID_FILE"
	CodeSecretUnknown = "SECRET.UNKNOWN"
	MaxSecretBytes    = 1 << 20 // 1 MiB
)

// Ref is one --secret-file id=path binding.
type Ref struct {
	ID   string
	Path string // operator path; never echoed into plan result as secret content
}

// ParsePairs parses id=path pairs.
func ParsePairs(pairs []string) ([]Ref, []diagnostic.Diagnostic) {
	var out []Ref
	var diags []diagnostic.Diagnostic
	seen := map[string]bool{}
	for _, p := range pairs {
		id, path, ok := strings.Cut(p, "=")
		if !ok || id == "" || path == "" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
				"secret-file requires id=path").WithHint("Example: --secret-file db_password=/run/secrets/db"))
			continue
		}
		if seen[id] {
			diags = append(diags, diagnostic.Error(CodeSecretInvalid, "duplicate secret-file id").
				WithSource("", "/secrets/"+id, 0, 0))
			continue
		}
		seen[id] = true
		out = append(out, Ref{ID: id, Path: path})
	}
	return out, diags
}

// Validate checks refs against declared secrets: regular no-follow file, readable, <=1MiB.
// Diagnostics never include file contents.
func Validate(declared map[string]ir.Secret, refs []Ref) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	provided := map[string]Ref{}
	for _, r := range refs {
		provided[r.ID] = r
		if _, ok := declared[r.ID]; !ok {
			diags = append(diags, diagnostic.Error(CodeSecretUnknown,
				fmt.Sprintf("secret %q is not declared in the deployment", r.ID)).
				WithSource("", "/secrets/"+r.ID, 0, 0))
			continue
		}
		diags = append(diags, validateFile(r)...)
	}
	ids := make([]string, 0, len(declared))
	for id, s := range declared {
		if s.Required {
			ids = append(ids, id)
		}
	}
	// stable order
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		if _, ok := provided[id]; !ok {
			diags = append(diags, diagnostic.Error(CodeSecretMissing,
				fmt.Sprintf("required secret %q has no file", id)).
				WithSource("", "/secrets/"+id, 0, 0).
				WithHint("Pass --secrets-dir <dir> or --secret-file "+id+"=<path>"))
		}
	}
	diagnostic.SortDeterministic(diags)
	return diags
}

func validateFile(r Ref) []diagnostic.Diagnostic {
	// Open with O_NOFOLLOW when available via OpenFile + Lstat check.
	fi, err := os.Lstat(r.Path)
	if err != nil {
		return []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid,
			fmt.Sprintf("secret %q path is not readable", r.ID)).
			WithSource("", "/secrets/"+r.ID, 0, 0).
			WithHint("Provide a regular file path for --secret-file")}
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid,
			fmt.Sprintf("secret %q must not be a symlink", r.ID)).
			WithSource("", "/secrets/"+r.ID, 0, 0)}
	}
	if !fi.Mode().IsRegular() {
		return []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid,
			fmt.Sprintf("secret %q must be a regular file", r.ID)).
			WithSource("", "/secrets/"+r.ID, 0, 0)}
	}
	if fi.Size() > MaxSecretBytes {
		return []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid,
			fmt.Sprintf("secret %q exceeds 1 MiB", r.ID)).
			WithSource("", "/secrets/"+r.ID, 0, 0)}
	}
	// Prove readability without retaining content in diagnostics.
	f, err := os.OpenFile(r.Path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		// Fallback without O_NOFOLLOW if unsupported; still reject symlinks via Lstat above.
		f, err = os.Open(r.Path)
		if err != nil {
			return []diagnostic.Diagnostic{diagnostic.Error(CodeSecretInvalid,
				fmt.Sprintf("secret %q cannot be opened", r.ID)).
				WithSource("", "/secrets/"+r.ID, 0, 0)}
		}
	}
	defer f.Close()
	var buf [1]byte
	_, _ = f.Read(buf[:]) // touch; discard
	// Never put path basename content or bytes into message beyond id.
	_ = filepath.Base(r.Path)
	return nil
}
