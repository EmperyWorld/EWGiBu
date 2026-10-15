package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
)

// Diagnostic codes for bundle verification.
const (
	CodeBundleParse     = "BUNDLE.PARSE_ERROR"
	CodeBundlePath      = "BUNDLE.INVALID_PATH"
	CodeBundleDuplicate = "BUNDLE.DUPLICATE_ENTRY"
	CodeBundleType      = "BUNDLE.UNSUPPORTED_TYPE"
	CodeBundleQuota     = "BUNDLE.QUOTA"
	CodeBundleHash      = "BUNDLE.HASH_MISMATCH"
	CodeBundleMeta      = "BUNDLE.META_INVALID"
	CodeBundleMissing   = "BUNDLE.MISSING_ENTRY"
	CodeBundleCollision = "BUNDLE.PATH_COLLISION"
)

// Quotas are verifier limits.
type Quotas struct {
	MaxEntries       int
	MaxEntryBytes    int64
	MaxExpandedBytes int64
	MaxRatio         int
	MaxPathBytes     int
}

// DefaultQuotas returns TZ defaults.
func DefaultQuotas() Quotas {
	return Quotas{
		MaxEntries: DefaultMaxEntries, MaxEntryBytes: DefaultMaxEntryBytes,
		MaxExpandedBytes: DefaultMaxExpandedBytes, MaxRatio: DefaultMaxRatio,
		MaxPathBytes: DefaultMaxPathBytes,
	}
}

// Entry is one verified archive member.
type Entry struct {
	Path    string
	Mode    string
	Size    int64
	SHA256  string
	Content []byte // nil for inspect-without-payload mode if skipped
}

// Result is a successful verification.
type Result struct {
	Meta           Meta
	Entries        map[string]Entry
	Order          []string
	CompressedSize int64
	ExpandedSize   int64
}

// VerifyOptions configures verification.
type VerifyOptions struct {
	Quotas       Quotas
	KeepContents bool // keep file bytes in Result.Entries (inspect may omit payload)
}

// Verify inspects a hostile archive stream without extracting to disk.
func Verify(r io.Reader, opts VerifyOptions) (Result, []diagnostic.Diagnostic) {
	if opts.Quotas.MaxEntries == 0 {
		opts.Quotas = DefaultQuotas()
	}
	var diags []diagnostic.Diagnostic
	limited := &countReader{r: r}
	gr, err := gzip.NewReader(limited)
	if err != nil {
		return Result{}, []diagnostic.Diagnostic{diagnostic.Error(CodeBundleParse, "gzip: "+err.Error())}
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	seen := map[string]bool{}
	fold := map[string]string{}
	entries := map[string]Entry{}
	var order []string
	var expanded int64
	var metaRaw []byte
	nEntries := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			diags = append(diags, diagnostic.Error(CodeBundleParse, "tar: "+err.Error()))
			return Result{}, diags
		}
		nEntries++
		if nEntries > opts.Quotas.MaxEntries || nEntries > HardMaxEntries {
			diags = append(diags, diagnostic.Error(CodeBundleQuota, "entry count exceeds quota"))
			return Result{}, diags
		}
		name := hdr.Name
		if err := validateArchivePath(name, opts.Quotas.MaxPathBytes); err != nil {
			diags = append(diags, diagnostic.Error(CodeBundlePath, err.Error()))
			return Result{}, diags
		}
		name = strings.TrimSuffix(name, "/")
		if seen[name] {
			diags = append(diags, diagnostic.Error(CodeBundleDuplicate, "duplicate entry "+name))
			return Result{}, diags
		}
		ck := strings.ToLower(name)
		if prev, ok := fold[ck]; ok && prev != name {
			diags = append(diags, diagnostic.Error(CodeBundleCollision, "case-folding collision "+prev+" vs "+name))
			return Result{}, diags
		}
		for _, r := range name {
			if unicode.Is(unicode.M, r) {
				diags = append(diags, diagnostic.Error(CodeBundlePath, "combining characters forbidden in path"))
				return Result{}, diags
			}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			seen[name] = true
			fold[ck] = name
			continue
		case tar.TypeReg:
			// ok
		case tar.TypeSymlink, tar.TypeLink:
			diags = append(diags, diagnostic.Error(CodeBundleType, "links are forbidden in bundles"))
			return Result{}, diags
		default:
			diags = append(diags, diagnostic.Error(CodeBundleType, fmt.Sprintf("unsupported tar type %v for %s", hdr.Typeflag, name)))
			return Result{}, diags
		}
		if hdr.Size < 0 || hdr.Size > opts.Quotas.MaxEntryBytes {
			diags = append(diags, diagnostic.Error(CodeBundleQuota, "entry size exceeds quota: "+name))
			return Result{}, diags
		}
		if expanded+hdr.Size > opts.Quotas.MaxExpandedBytes || expanded+hdr.Size > HardMaxExpandedBytes {
			diags = append(diags, diagnostic.Error(CodeBundleQuota, "expanded size exceeds quota"))
			return Result{}, diags
		}
		mode := hdr.FileInfo().Mode().Perm()
		if mode != 0o644 && mode != 0o755 {
			// allow only normalized modes
			diags = append(diags, diagnostic.Error(CodeBundleType, fmt.Sprintf("invalid mode %o for %s", mode, name)))
			return Result{}, diags
		}

		h := sha256.New()
		var buf bytes.Buffer
		lr := io.LimitReader(tr, hdr.Size+1)
		var w io.Writer = h
		if opts.KeepContents || name == "bundle.json" {
			w = io.MultiWriter(h, &buf)
		}
		n, err := io.Copy(w, lr)
		if err != nil {
			diags = append(diags, diagnostic.Error(CodeBundleParse, "read entry: "+err.Error()))
			return Result{}, diags
		}
		if n != hdr.Size {
			diags = append(diags, diagnostic.Error(CodeBundleParse, "truncated entry: "+name))
			return Result{}, diags
		}
		expanded += n
		sum := hex.EncodeToString(h.Sum(nil))
		modeStr := "0644"
		if mode&0o111 != 0 {
			modeStr = "0755"
		}
		ent := Entry{Path: name, Mode: modeStr, Size: n, SHA256: sum}
		if opts.KeepContents || name == "bundle.json" {
			// An empty file is a real zero-byte payload, not a missing read.
			ent.Content = append([]byte{}, buf.Bytes()...)
		}
		if name == "bundle.json" {
			metaRaw = ent.Content
		}
		seen[name] = true
		fold[ck] = name
		entries[name] = ent
		order = append(order, name)
	}

	compSize := limited.n
	if compSize > 0 && opts.Quotas.MaxRatio > 0 {
		if expanded/compSize > int64(opts.Quotas.MaxRatio) {
			diags = append(diags, diagnostic.Error(CodeBundleQuota, "compression ratio exceeds quota"))
			return Result{}, diags
		}
	}
	if len(metaRaw) == 0 {
		diags = append(diags, diagnostic.Error(CodeBundleMissing, "bundle.json missing"))
		return Result{}, diags
	}
	var meta Meta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		diags = append(diags, diagnostic.Error(CodeBundleMeta, "bundle.json: "+err.Error()))
		return Result{}, diags
	}
	if meta.FormatVersion != FormatVersion {
		diags = append(diags, diagnostic.Error(CodeBundleMeta, "unsupported formatVersion "+meta.FormatVersion))
		return Result{}, diags
	}
	// Verify entry digests listed in meta.
	listed := map[string]MetaEntry{}
	for _, e := range meta.Entries {
		listed[e.Path] = e
	}
	for _, e := range meta.Entries {
		got, ok := entries[e.Path]
		if !ok {
			diags = append(diags, diagnostic.Error(CodeBundleMissing, "listed entry missing: "+e.Path))
			continue
		}
		if got.SHA256 != e.SHA256 || got.Size != e.Size {
			diags = append(diags, diagnostic.Error(CodeBundleHash, "hash/size mismatch for "+e.Path))
		}
	}
	for path := range entries {
		if path == "bundle.json" {
			continue
		}
		if _, ok := listed[path]; !ok {
			diags = append(diags, diagnostic.Error(CodeBundleMeta, "archive entry not listed in bundle.json: "+path))
		}
	}
	// Re-canonicalize meta without self-hash check; ensure deterministic parse.
	if _, err := canonicaljson.Marshal(meta); err != nil {
		diags = append(diags, diagnostic.Error(CodeBundleMeta, "meta canonicalization failed"))
	}
	diagnostic.SortDeterministic(diags)
	if diagnostic.HasError(diags) {
		return Result{}, diags
	}
	return Result{
		Meta: meta, Entries: entries, Order: order,
		CompressedSize: compSize, ExpandedSize: expanded,
	}, nil
}

func validateArchivePath(name string, maxBytes int) error {
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

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// ReadFile returns entry bytes from a verified result.
func (r Result) ReadFile(path string) ([]byte, error) {
	e, ok := r.Entries[path]
	if !ok {
		return nil, fmt.Errorf("missing %s", path)
	}
	if e.Content == nil {
		return nil, fmt.Errorf("content not retained for %s", path)
	}
	return e.Content, nil
}
