package workspaceingest

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/diagnostic"
	"iacpack/internal/inputs"
)

// Limits for explicit authoring uploads (import UI / workspace ingest).
type Limits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxPathBytes  int
}

func DefaultLimits() Limits {
	return Limits{
		MaxFiles:      10000,
		MaxFileBytes:  32 << 20,
		MaxTotalBytes: 64 << 20,
		MaxPathBytes:  255,
	}
}

type FilePart struct {
	Path string
	Data []byte
}

type Request struct {
	Mode      string // file | tree | zip
	Target    string // workspace-relative file or directory root
	Overwrite bool
	File      []byte
	Tree      []FilePart
}

type Result struct {
	Written []string
	Skipped []string
}

func Ingest(workspace string, req Request, lim Limits) (Result, []diagnostic.Diagnostic) {
	var out Result
	var diags []diagnostic.Diagnostic
	if lim.MaxFiles == 0 {
		lim = DefaultLimits()
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "workspace is required")}
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	target, err := cleanWorkspaceRel(req.Target, lim.MaxPathBytes)
	if err != nil {
		return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
	}
	switch mode {
	case "file":
		if len(req.File) == 0 {
			return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "file content is empty")}
		}
		if int64(len(req.File)) > lim.MaxFileBytes {
			return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "file exceeds size limit")}
		}
		w, sk, d := writeOneFile(workspace, target, req.File, req.Overwrite, lim)
		diags = append(diags, d...)
		if w != "" {
			out.Written = append(out.Written, w)
		}
		if sk != "" {
			out.Skipped = append(out.Skipped, sk)
		}
	case "tree":
		parts, td := normalizeTreeParts(target, req.Tree, lim)
		diags = append(diags, td...)
		if diagnostic.HasError(diags) {
			return out, diags
		}
		for _, p := range parts {
			w, sk, d := writeOneFile(workspace, p.path, p.data, req.Overwrite, lim)
			diags = append(diags, d...)
			if diagnostic.HasError(diags) {
				return out, diags
			}
			if w != "" {
				out.Written = append(out.Written, w)
			}
			if sk != "" {
				out.Skipped = append(out.Skipped, sk)
			}
		}
	case "zip", "tar":
		if len(req.File) == 0 {
			return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip archive is empty")}
		}
		var extracted []FilePart
		var zd []diagnostic.Diagnostic
		if strings.EqualFold(req.Mode, "tar") {
			extracted, zd = extractTarGz(req.File, lim)
		} else {
			extracted, zd = extractZip(req.File, lim)
		}
		diags = append(diags, zd...)
		if diagnostic.HasError(diags) {
			return out, diags
		}
		parts, td := normalizeTreeParts(target, extracted, lim)
		diags = append(diags, td...)
		if diagnostic.HasError(diags) {
			return out, diags
		}
		for _, p := range parts {
			w, sk, d := writeOneFile(workspace, p.path, p.data, req.Overwrite, lim)
			diags = append(diags, d...)
			if diagnostic.HasError(diags) {
				return out, diags
			}
			if w != "" {
				out.Written = append(out.Written, w)
			}
			if sk != "" {
				out.Skipped = append(out.Skipped, sk)
			}
		}
	default:
		return out, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("unknown upload mode %q", req.Mode))}
	}
	sort.Strings(out.Written)
	sort.Strings(out.Skipped)
	return out, diags
}

type treePart struct {
	path string
	data []byte
}

func normalizeTreeParts(root string, parts []FilePart, lim Limits) ([]treePart, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if len(parts) > lim.MaxFiles {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "too many files in upload")}
	}
	var total int64
	var out []treePart
	seen := map[string]struct{}{}
	for _, f := range parts {
		rel := strings.TrimSpace(f.Path)
		rel = strings.TrimPrefix(rel, "./")
		rel = filepath.ToSlash(rel)
		if rel == "" {
			continue
		}
		if err := validateMemberPath(rel, lim.MaxPathBytes); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error()))
			continue
		}
		full := rel
		if root != "" && root != "." {
			full = filepath.ToSlash(filepath.Join(root, rel))
		}
		full, err := cleanWorkspaceRel(full, lim.MaxPathBytes)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error()))
			continue
		}
		if _, ok := seen[full]; ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportCollision, "duplicate upload path "+full))
			continue
		}
		seen[full] = struct{}{}
		if int64(len(f.Data)) > lim.MaxFileBytes {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "file exceeds size limit: "+full))
			continue
		}
		total += int64(len(f.Data))
		if total > lim.MaxTotalBytes {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "upload exceeds total size limit"))
			break
		}
		out = append(out, treePart{path: full, data: f.Data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, diags
}

func writeOneFile(workspace, rel string, data []byte, overwrite bool, lim Limits) (written, skipped string, diags []diagnostic.Diagnostic) {
	rel, err := cleanWorkspaceRel(rel, lim.MaxPathBytes)
	if err != nil {
		return "", "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
	}
	if err := rejectUnsafePath(rel); err != nil {
		return "", "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPolicyDenied, err.Error())}
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	if !underRoot(workspace, abs) {
		return "", "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "path escapes workspace")}
	}
	if _, err := os.Lstat(abs); err == nil {
		if !overwrite {
			return "", rel, nil
		}
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "mkdir: "+err.Error())}
	}
	afs := filesystem.OS{}
	var errWrite error
	if overwrite {
		errWrite = afs.WriteFileAtomicReplace(abs, data, 0o644)
	} else {
		errWrite = afs.WriteFileAtomic(abs, data, 0o644)
	}
	if errWrite != nil {
		return "", "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, errWrite.Error())}
	}
	return rel, "", nil
}

func cleanWorkspaceRel(p string, maxPathBytes int) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || strings.Contains(p, "\x00") {
		return "", fmt.Errorf("path must be workspace-relative")
	}
	if err := inputs.ValidateRelPath(filepath.ToSlash(p)); err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("path escapes workspace")
	}
	if len(clean) > maxPathBytes {
		return "", fmt.Errorf("path too long")
	}
	return clean, nil
}

func validateMemberPath(name string, maxBytes int) error {
	if name == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return fmt.Errorf("path contains NUL or backslash")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == ".." {
		return fmt.Errorf("absolute or traversal path %q", name)
	}
	clean := strings.TrimSuffix(name, "/")
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid path segment in %q", name)
		}
	}
	if len(name) > maxBytes {
		return fmt.Errorf("path exceeds %d bytes", maxBytes)
	}
	return nil
}

func rejectUnsafePath(rel string) error {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(rel)))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return fmt.Errorf("refusing to write .env files; use placeholders only")
	}
	low := strings.ToLower(rel)
	if strings.Contains(low, "docker.sock") {
		return fmt.Errorf("docker.sock paths forbidden")
	}
	return nil
}

func underRoot(root, abs string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(abs))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

type UploadFilePart struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
}

// DecodeBase64Files decodes API file parts.
func DecodeBase64Files(parts []UploadFilePart) ([]FilePart, []diagnostic.Diagnostic) {
	var out []FilePart
	var diags []diagnostic.Diagnostic
	for _, p := range parts {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(p.ContentBase64))
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError,
				fmt.Sprintf("invalid base64 for %q", p.Path)))
			continue
		}
		out = append(out, FilePart{Path: p.Path, Data: raw})
	}
	return out, diags
}

func (r Result) MapForAPI() map[string]any {
	return map[string]any{
		"written": stringSliceOrEmpty(r.Written),
		"skipped": stringSliceOrEmpty(r.Skipped),
	}
}

func stringSliceOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
