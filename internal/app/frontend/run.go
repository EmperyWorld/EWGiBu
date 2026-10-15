package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/inputs"
)

// Runner executes a package-manager argv in dir. argv[0] is the binary.
type Runner interface {
	Run(dir string, argv []string) error
}

// ExecRunner runs the package manager on PATH. Install may use the network.
type ExecRunner struct{}

func (ExecRunner) Run(dir string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty package-manager argv")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		// Keep pnpm temp files on the same disk as the project copy.
		"TMPDIR=" + dir,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := clipCommandOutput(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", strings.Join(argv, " "), msg)
	}
	return nil
}

// clipCommandOutput keeps the tail of a package-manager log. The non-zero
// exit reason is at the end; the head is progress and warnings.
func clipCommandOutput(s string) string {
	const limit = 4096
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return "…\n" + s[len(s)-limit:]
}

// File is one file from a package build output.
type File struct {
	Rel  string
	Mode os.FileMode
	Body []byte
}

// Materialize runs each package build in a temp copy of the context and returns
// output files keyed by build id. An empty output or a missing entry file is an error.
// A cache hit requires a non-empty previous output that contains the entry file.
func Materialize(ws inputs.FS, builds map[string]domain.PackageBuild, runner Runner, cacheRoot string) (map[string][]File, []diagnostic.Diagnostic) {
	if len(builds) == 0 {
		return nil, nil
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	var diags []diagnostic.Diagnostic
	out := map[string][]File{}
	ids := make([]string, 0, len(builds))
	for id := range builds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		files, err := one(ws, id, builds[id], runner, cacheRoot)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend, err.Error()))
			continue
		}
		out[id] = files
	}
	return out, diags
}

func one(ws inputs.FS, id string, b domain.PackageBuild, runner Runner, cacheRoot string) ([]File, error) {
	copied, err := copyContext(ws, id, b)
	if err != nil {
		return nil, fmt.Errorf("package build %s: %w", id, err)
	}
	key := hashFiles(copied)
	if hit, ok := readCache(cacheRoot, key, b.Entry); ok {
		return hit, nil
	}
	tmp, copyImport, err := tempBuildDir(b.PackageManager)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := writeCopy(tmp, copied); err != nil {
		return nil, fmt.Errorf("package build %s: %w", id, err)
	}
	runDir := tmp
	ctx := strings.Trim(b.Context.String(), "/")
	if ctx != "" && ctx != "." {
		runDir = filepath.Join(tmp, filepath.FromSlash(ctx))
	}
	install, script := managerArgv(b)
	if copyImport && b.PackageManager == "pnpm" {
		install = append(install, "--package-import-method=copy")
	}
	if err := runner.Run(runDir, install); err != nil {
		return nil, fmt.Errorf("package build %s: %w", id, err)
	}
	if err := runner.Run(runDir, script); err != nil {
		return nil, fmt.Errorf("package build %s: %w", id, err)
	}
	files, err := readOutput(runDir, b.Output, b.Entry)
	if err != nil {
		return nil, fmt.Errorf("package build %s: %w", id, err)
	}
	_ = writeCache(cacheRoot, key, files)
	return files, nil
}

type copiedFile struct {
	Rel  string
	Mode os.FileMode
	Body []byte
}

func copyContext(ws inputs.FS, id string, b domain.PackageBuild) ([]copiedFile, error) {
	ctx := strings.Trim(b.Context.String(), "/")
	roots := []string{}
	if ctx != "" && ctx != "." {
		roots = append(roots, ctx)
	} else {
		roots = append(roots, ".")
	}
	if b.PackageManager == "pnpm" {
		extra, diags := outsideRoots(ws, id, ctx)
		if len(diags) > 0 {
			return nil, fmt.Errorf("%s", diags[0].Message)
		}
		roots = append(roots, extra...)
	}
	srcFiles, srcDirs, srcDiags := sourceOutside(ws, id, ctx)
	if len(srcDiags) > 0 {
		return nil, fmt.Errorf("%s", srcDiags[0].Message)
	}
	roots = append(roots, srcDirs...)
	var out []copiedFile
	seen := map[string]struct{}{}
	for _, root := range roots {
		files, err := walkRoot(ws, ctx, b.Output, root)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, ok := seen[f.Rel]; ok {
				continue
			}
			seen[f.Rel] = struct{}{}
			out = append(out, f)
		}
	}
	for _, rel := range srcFiles {
		if _, ok := seen[rel]; ok {
			continue
		}
		info, err := ws.Lstat(rel)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		body, err := ws.ReadFile(rel)
		if err != nil {
			if isSymlinkOpenErr(err) {
				continue
			}
			return nil, err
		}
		seen[rel] = struct{}{}
		out = append(out, copiedFile{Rel: rel, Mode: 0o644, Body: body})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("context %s is empty", b.Context.String())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

func walkRoot(ws inputs.FS, ctx, output, root string) ([]copiedFile, error) {
	var out []copiedFile
	var walk func(rel string) error
	walk = func(rel string) error {
		entries, err := ws.ReadDir(rel)
		if err != nil {
			return err
		}
		names := make([]inputs.DirEntry, len(entries))
		copy(names, entries)
		sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
		for _, e := range names {
			name := e.Name()
			if name == "." || name == ".." || name == "node_modules" || name == ".git" {
				continue
			}
			if name == ".env" || strings.HasPrefix(name, ".env.") {
				continue
			}
			child := name
			if rel != "." {
				child = path.Join(rel, name)
			}
			if e.IsDir() {
				if isSymlink(e) {
					continue
				}
				if insideContext(ctx, child) && (child == path.Join(ctx, output) || strings.HasPrefix(child, path.Join(ctx, output)+"/")) {
					continue
				}
				if err := walk(child); err != nil {
					return err
				}
				continue
			}
			if isSymlink(e) {
				continue
			}
			body, err := ws.ReadFile(child)
			if err != nil {
				if isSymlinkOpenErr(err) {
					continue
				}
				return err
			}
			mode := os.FileMode(0o644)
			if info, err := e.Info(); err == nil && info.Mode()&0o111 != 0 {
				mode = 0o755
			}
			out = append(out, copiedFile{Rel: child, Mode: mode, Body: body})
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return out, nil
}

// tempBuildDir places the pnpm project copy on the same filesystem as the
// pnpm store so hardlinks work. When that directory cannot be created, the
// caller installs with package-import-method=copy.
func tempBuildDir(manager string) (dir string, copyImport bool, err error) {
	store := ""
	if manager == "pnpm" {
		store = pnpmStorePath()
	}
	return tempBuildDirAt(manager, store)
}

func tempBuildDirAt(manager, store string) (dir string, copyImport bool, err error) {
	if manager == "pnpm" && store != "" {
		parent := storeTempParent(store)
		if parent != "" {
			if d, err := os.MkdirTemp(parent, ".iacpack-spa-"); err == nil {
				if sameDevice(d, store) {
					return d, false, nil
				}
				_ = os.RemoveAll(d)
			}
		}
		d, err := os.MkdirTemp("", "iacpack-spa-")
		return d, true, err
	}
	d, err := os.MkdirTemp("", "iacpack-spa-")
	return d, false, err
}

func storeTempParent(store string) string {
	parent := filepath.Dir(store)
	base := filepath.Base(store)
	if len(base) > 1 && base[0] == 'v' {
		digits := true
		for _, r := range base[1:] {
			if r < '0' || r > '9' {
				digits = false
				break
			}
		}
		if digits {
			parent = filepath.Dir(parent)
		}
	}
	return parent
}

func pnpmStorePath() string {
	cmd := exec.Command("pnpm", "store", "path")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sameDevice(a, b string) bool {
	da, oka := deviceOf(a)
	db, okb := deviceOf(b)
	return oka && okb && da == db
}

func deviceOf(p string) (uint64, bool) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

func isSymlink(e inputs.DirEntry) bool {
	if e.Type()&os.ModeSymlink != 0 {
		return true
	}
	info, err := e.Info()
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func isSymlinkOpenErr(err error) bool {
	return errors.Is(err, syscall.ELOOP) || strings.Contains(err.Error(), "too many levels of symbolic links")
}

func hashFiles(files []copiedFile) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%x\n", f.Rel, sha256.Sum256(f.Body))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeCopy(dir string, files []copiedFile) error {
	for _, f := range files {
		dest := filepath.Join(dir, filepath.FromSlash(f.Rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, f.Body, f.Mode); err != nil {
			return err
		}
		if err := os.Chmod(dest, f.Mode); err != nil {
			return err
		}
	}
	return nil
}

func managerArgv(b domain.PackageBuild) (install, script []string) {
	switch b.PackageManager {
	case "pnpm":
		return []string{"pnpm", "install", "--frozen-lockfile"}, []string{"pnpm", "run", b.Script}
	case "npm":
		return []string{"npm", "ci"}, []string{"npm", "run", b.Script}
	case "yarn":
		return []string{"yarn", "install", "--frozen-lockfile"}, []string{"yarn", "run", b.Script}
	case "bun":
		return []string{"bun", "install", "--frozen-lockfile"}, []string{"bun", "run", b.Script}
	default:
		return []string{b.PackageManager, "install"}, []string{b.PackageManager, "run", b.Script}
	}
}

func readOutput(root, output, entry string) ([]File, error) {
	base := filepath.Join(root, filepath.FromSlash(output))
	var files []File
	err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "..") {
			return fmt.Errorf("output path escaped")
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		files = append(files, File{Rel: rel, Mode: mode, Body: body})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("output %s: %w", output, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	if len(files) == 0 {
		return nil, fmt.Errorf("output %s is empty", output)
	}
	found := false
	for _, f := range files {
		if f.Rel == entry && len(f.Body) > 0 {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("output %s has no non-empty %s", output, entry)
	}
	return files, nil
}

func readCache(root, key, entry string) ([]File, bool) {
	if root == "" || key == "" {
		return nil, false
	}
	dir := filepath.Join(root, key)
	files, err := readOutput(dir, ".", entry)
	if err != nil || len(files) == 0 {
		return nil, false
	}
	return files, true
}

func writeCache(root, key string, files []File) error {
	if root == "" || key == "" {
		return nil
	}
	dir := filepath.Join(root, key)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, f := range files {
		dest := filepath.Join(dir, filepath.FromSlash(f.Rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, f.Body, f.Mode); err != nil {
			return err
		}
	}
	return nil
}
