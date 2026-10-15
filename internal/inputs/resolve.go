package inputs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/glob"
)

// FileRecord is one locked input file (workspace-relative slash path).
type FileRecord struct {
	Path   string // workspace-relative UTF-8 path
	Size   int64
	Mode   string // normalized octal like "0644"
	SHA256 string // lowercase hex
	Source string // resource id or "image/<id>/build"
}

// ResolveOptions configures declared-input resolution.
type ResolveOptions struct {
	// OutputRel, if non-empty, is a workspace-relative path that must not be
	// included as an input (output-inside-input rejection).
	OutputRel string
}

// ResolveProject expands all declared resources and image build contexts.
func ResolveProject(ws FS, project domain.Project, opts ResolveOptions) ([]FileRecord, []diagnostic.Diagnostic) {
	var (
		diags   []diagnostic.Diagnostic
		records []FileRecord
		seen    = map[string]struct{}{} // path + source already kept
		paths   = map[string]struct{}{} // workspace path already accepted
		fold    = map[string]string{}   // collision key -> path
	)

	resIDs := sortedKeys(project.Spec.Resources)
	for _, id := range resIDs {
		r := project.Spec.Resources[id]
		pointer := "/spec/resources/" + id
		got, d := resolveTree(ws, r.Source.String(), r.Include, r.Exclude, r.AllowEmpty, r.Mode, "resource/"+id, pointer, opts.OutputRel)
		diags = append(diags, d...)
		for _, rec := range got {
			keep, td := track(seen, paths, fold, rec)
			diags = append(diags, td...)
			if keep {
				records = append(records, rec)
			}
		}
	}

	dbIDs := sortedKeys(project.Spec.Databases)
	for _, id := range dbIDs {
		db := project.Spec.Databases[id]
		pointer := "/spec/databases/" + id + "/migrations"
		got, d := resolveTree(ws, db.Migrations, []string{"**/*.sql", "*.sql"}, nil, false, "", "database/"+id, pointer, opts.OutputRel)
		diags = append(diags, d...)
		for _, rec := range got {
			keep, td := track(seen, paths, fold, rec)
			diags = append(diags, td...)
			if keep {
				records = append(records, rec)
			}
		}
	}

	imgIDs := sortedKeys(project.Spec.Images)
	for _, id := range imgIDs {
		img := project.Spec.Images[id]
		if img.Build == nil {
			continue
		}
		b := img.Build
		pointer := "/spec/images/" + id + "/build"
		src := b.Context.String()
		got, d := resolveTree(ws, src, b.Include, b.Exclude, false, "", "image/"+id+"/build", pointer, opts.OutputRel)
		diags = append(diags, d...)
		dfRel, err := JoinRel(src, b.Dockerfile.String())
		if err != nil {
			diags = append(diags, diagnostic.Error(CodePathOutsideWorkspace,
				fmt.Sprintf("dockerfile path invalid: %v", err)).
				WithSource("", pointer+"/dockerfile", 0, 0))
		}
		foundDF := false
		for _, rec := range got {
			if dfRel != "" && rec.Path == dfRel {
				foundDF = true
			}
			keep, td := track(seen, paths, fold, rec)
			diags = append(diags, td...)
			if keep {
				records = append(records, rec)
			}
		}
		if dfRel != "" && !foundDF && !diagnostic.HasError(d) {
			diags = append(diags, diagnostic.Error(CodeInputMissing,
				fmt.Sprintf("dockerfile %q not found inside build context %q", b.Dockerfile, b.Context)).
				WithSource("", pointer+"/dockerfile", 0, 0))
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Path != records[j].Path {
			return records[i].Path < records[j].Path
		}
		return records[i].Source < records[j].Source
	})
	diagnostic.SortDeterministic(diags)
	return records, diags
}

func track(seen, paths map[string]struct{}, fold map[string]string, rec FileRecord) (bool, []diagnostic.Diagnostic) {
	key := rec.Path + "\x00" + rec.Source
	if _, ok := seen[key]; ok {
		return false, nil
	}
	if _, ok := paths[rec.Path]; ok {
		// Same workspace file, another resource. Not a collision: each source
		// needs its own payload copy.
		seen[key] = struct{}{}
		return true, nil
	}
	ck := CollisionKey(rec.Path)
	if prev, ok := fold[ck]; ok && prev != rec.Path {
		return false, []diagnostic.Diagnostic{diagnostic.Error(CodeInputCollision,
			fmt.Sprintf("case-folding collision between %q and %q", prev, rec.Path))}
	}
	seen[key] = struct{}{}
	paths[rec.Path] = struct{}{}
	fold[ck] = rec.Path
	return true, nil
}

func resolveTree(ws FS, source string, include, exclude []string, allowEmpty bool, modeDecl, sourceID, pointer, outputRel string) ([]FileRecord, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if err := ValidateRelPath(source); err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(CodePathOutsideWorkspace,
			fmt.Sprintf("invalid source path %q: %v", source, err)).
			WithSource("", pointer+"/source", 0, 0).
			WithHint("Use a path relative to the workspace root")}
	}
	if len(include) == 0 {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeInputEmpty,
			"include patterns must be non-empty").
			WithSource("", pointer+"/include", 0, 0)}
	}
	for i, p := range include {
		if err := glob.Validate(p); err != nil {
			diags = append(diags, diagnostic.Error(CodeInputGlob,
				fmt.Sprintf("invalid include pattern %q: %v", p, err)).
				WithSource("", fmt.Sprintf("%s/include/%d", pointer, i), 0, 0))
		}
	}
	for i, p := range exclude {
		if err := glob.Validate(p); err != nil {
			diags = append(diags, diagnostic.Error(CodeInputGlob,
				fmt.Sprintf("invalid exclude pattern %q: %v", p, err)).
				WithSource("", fmt.Sprintf("%s/exclude/%d", pointer, i), 0, 0))
		}
	}
	if diagnostic.HasError(diags) {
		return nil, diags
	}

	info, err := ws.Lstat(source)
	if err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeInputMissing,
			fmt.Sprintf("source %q not found: %v", source, err)).
			WithSource("", pointer+"/source", 0, 0)}
	}
	open, info, isDir, spec := classifyPath(ws, source, info)
	if spec != nil {
		d := spec.WithSource("", pointer+"/source", 0, 0)
		return nil, []diagnostic.Diagnostic{d}
	}
	var matched []selectedFile
	if !isDir {
		name := source
		if i := strings.LastIndex(source, "/"); i >= 0 {
			name = source[i+1:]
		}
		ok, err := matchesAny(include, name)
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeInputGlob, err.Error())}
		}
		exc, err := matchesAny(exclude, name)
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeInputGlob, err.Error())}
		}
		if ok && !exc {
			matched = []selectedFile{{path: source, open: open}}
		}
	} else if allLiteralIncludes(include) {
		matched, diags = literalFiles(ws, source, include, exclude, outputRel, pointer, diags)
	} else {
		matched, diags = walkIncluded(ws, source, include, exclude, outputRel, pointer, diags)
	}
	if diagnostic.HasError(diags) {
		return nil, diags
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].path < matched[j].path })
	if len(matched) == 0 {
		if allowEmpty {
			return nil, diags
		}
		return nil, append(diags, diagnostic.Error(CodeInputEmpty,
			fmt.Sprintf("include matched no files under %q", source)).
			WithSource("", pointer+"/include", 0, 0).
			WithHint("Add files, widen include, or set allowEmpty: true"))
	}

	out := make([]FileRecord, 0, len(matched))
	for _, p := range matched {
		rec, d := hashFile(ws, p.path, p.open, modeDecl, sourceID)
		if d != nil {
			diags = append(diags, *d)
			continue
		}
		out = append(out, rec)
	}
	return out, diags
}

type selectedFile struct {
	path string
	open string
}

func allLiteralIncludes(include []string) bool {
	for _, p := range include {
		if strings.ContainsAny(p, "*?") {
			return false
		}
	}
	return true
}

func literalFiles(ws FS, source string, include, exclude []string, outputRel, pointer string, diags []diagnostic.Diagnostic) ([]selectedFile, []diagnostic.Diagnostic) {
	var matched []selectedFile
	for _, pat := range include {
		rel, err := JoinRel(source, pat)
		if err != nil {
			diags = append(diags, diagnostic.Error(CodePathOutsideWorkspace,
				fmt.Sprintf("invalid include path %q: %v", pat, err)).
				WithSource("", pointer+"/include", 0, 0))
			continue
		}
		if outputRel != "" && (rel == outputRel || strings.HasPrefix(rel, outputRel+"/")) {
			diags = append(diags, diagnostic.Error(CodeInputOutputRecursion,
				fmt.Sprintf("input %q is inside output path %q", rel, outputRel)))
			continue
		}
		fi, err := ws.Lstat(rel)
		if err != nil {
			continue
		}
		open, info, isDir, spec := classifyPath(ws, rel, fi)
		if spec != nil {
			diags = append(diags, spec.WithSource("", pointer, 0, 0))
			continue
		}
		if isDir {
			continue
		}
		_ = info
		exc, err := matchesAny(exclude, pat)
		if err != nil {
			diags = append(diags, diagnostic.Error(CodeInputGlob, err.Error()))
			continue
		}
		if exc {
			continue
		}
		matched = append(matched, selectedFile{path: rel, open: open})
	}
	return matched, diags
}

func walkIncluded(ws FS, source string, include, exclude []string, outputRel, pointer string, diags []diagnostic.Diagnostic) ([]selectedFile, []diagnostic.Diagnostic) {
	var matched []selectedFile
	seenReal := map[string]struct{}{}
	var walk func(rel string)
	walk = func(rel string) {
		entries, err := ws.ReadDir(rel)
		if err != nil {
			diags = append(diags, diagnostic.Error(CodeInputIO,
				fmt.Sprintf("read directory %q: %v", rel, err)).
				WithSource("", pointer, 0, 0))
			return
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		sort.Strings(names)
		for _, name := range names {
			child, err := JoinRel(rel, name)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodePathOutsideWorkspace,
					fmt.Sprintf("invalid entry under %q: %v", rel, err)))
				continue
			}
			relTo := relUnder(source, child)
			inc, err := matchesAny(include, relTo)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeInputGlob, err.Error()))
				continue
			}
			descend, err := reachesAny(include, relTo)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeInputGlob, err.Error()))
				continue
			}
			if !inc && !descend {
				continue
			}
			fi, err := ws.Lstat(child)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeInputIO,
					fmt.Sprintf("lstat %q: %v", child, err)))
				continue
			}
			open, _, isDir, spec := classifyPath(ws, child, fi)
			if spec != nil {
				diags = append(diags, spec.WithSource("", pointer, 0, 0))
				continue
			}
			if isDir {
				if !descend {
					continue
				}
				if _, ok := seenReal[open]; ok {
					continue
				}
				seenReal[open] = struct{}{}
				walk(child)
				continue
			}
			if !inc {
				continue
			}
			if outputRel != "" && (child == outputRel || strings.HasPrefix(child, outputRel+"/")) {
				diags = append(diags, diagnostic.Error(CodeInputOutputRecursion,
					fmt.Sprintf("input %q is inside output path %q", child, outputRel)))
				continue
			}
			exc, err := matchesAny(exclude, relTo)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeInputGlob, err.Error()))
				continue
			}
			if exc {
				continue
			}
			matched = append(matched, selectedFile{path: child, open: open})
		}
	}
	rootOpen := source
	if followed, err := ws.FollowSymlink(source); err == nil && followed != "" {
		rootOpen = followed
	}
	seenReal[rootOpen] = struct{}{}
	walk(source)
	return matched, diags
}

func relUnder(source, cand string) string {
	if source == "" || source == "." {
		return cand
	}
	if cand == source {
		return ""
	}
	if strings.HasPrefix(cand, source+"/") {
		return strings.TrimPrefix(cand, source+"/")
	}
	return cand
}

func reachesAny(include []string, dir string) (bool, error) {
	for _, p := range include {
		ok, err := glob.Reaches(p, dir)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func matchesAny(patterns []string, name string) (bool, error) {
	for _, p := range patterns {
		ok, err := glob.Match(p, name)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func classifyPath(ws FS, rel string, fi FileInfo) (open string, info FileInfo, isDir bool, d *diagnostic.Diagnostic) {
	if err := rejectDevice(fi); err != nil {
		dd := diagnostic.Error(CodeInputForbiddenType, fmt.Sprintf("%q: %v", rel, err))
		return "", nil, false, &dd
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return rel, fi, fi.IsDir(), nil
	}
	target, err := ws.FollowSymlink(rel)
	if err != nil {
		dd := diagnostic.Error(CodeInputForbiddenType,
			fmt.Sprintf("%q: symlink escapes workspace", rel)).
			WithHint("Only symlinks whose target stays inside the workspace are accepted")
		return "", nil, false, &dd
	}
	tinfo, err := ws.Lstat(target)
	if err != nil {
		dd := diagnostic.Error(CodeInputIO, fmt.Sprintf("lstat %q: %v", target, err))
		return "", nil, false, &dd
	}
	if err := rejectDevice(tinfo); err != nil {
		dd := diagnostic.Error(CodeInputForbiddenType, fmt.Sprintf("%q: %v", rel, err))
		return "", nil, false, &dd
	}
	return target, tinfo, tinfo.IsDir(), nil
}

func rejectDevice(info FileInfo) error {
	m := info.Mode()
	if m&fs.ModeDevice != 0 || m&fs.ModeCharDevice != 0 {
		return fmt.Errorf("device files are forbidden in inputs")
	}
	if m&fs.ModeNamedPipe != 0 {
		return fmt.Errorf("FIFOs are forbidden in inputs")
	}
	if m&fs.ModeSocket != 0 {
		return fmt.Errorf("sockets are forbidden in inputs")
	}
	return nil
}

func normalizeMode(decl string, executable bool) string {
	if decl != "" {
		return decl
	}
	if executable {
		return "0755"
	}
	return "0644"
}

func hashFile(ws FS, rel, openRel, mode, sourceID string) (FileRecord, *diagnostic.Diagnostic) {
	if openRel == "" {
		openRel = rel
	}
	f, err := ws.Open(openRel)
	if err != nil {
		d := diagnostic.Error(CodeInputIO, fmt.Sprintf("open %q: %v", rel, err))
		return FileRecord{}, &d
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		d := diagnostic.Error(CodeInputIO, fmt.Sprintf("read %q: %v", rel, err))
		return FileRecord{}, &d
	}
	// Re-read size via Lstat for TOCTOU: size/mode must match hashed bytes.
	info, err := ws.Lstat(openRel)
	if err != nil {
		d := diagnostic.Error(CodeInputIO, fmt.Sprintf("lstat %q after hash: %v", rel, err))
		return FileRecord{}, &d
	}
	if info.Size() != n {
		d := diagnostic.Error(CodeInputChanged,
			fmt.Sprintf("file %q changed during hashing (size %d -> %d)", rel, n, info.Size())).
			WithHint("Ensure declared inputs are stable while locking")
		return FileRecord{}, &d
	}
	mode = normalizeMode(mode, info.Mode()&0o111 != 0)
	sum := hex.EncodeToString(h.Sum(nil))
	return FileRecord{
		Path:   rel,
		Size:   n,
		Mode:   mode,
		SHA256: sum,
		Source: sourceID,
	}, nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
