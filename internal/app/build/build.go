// Package build implements directory and bundle compilation.
// Package-manager builds in spec.builds run here; validate does not execute them.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/app/frontend"
	"iacpack/internal/app/validate"
	"iacpack/internal/artifact"
	"iacpack/internal/bundle"
	"iacpack/internal/canonicaljson"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/plan"
	"iacpack/internal/inputs"
	"iacpack/internal/lockfile"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/oci"
	"iacpack/internal/oci/layout"
	"iacpack/internal/params"
	"iacpack/internal/policy"
	"iacpack/internal/render/compose"
	"iacpack/internal/render/docs"
	"iacpack/internal/runtime/state"
)

// Writer materializes a verified virtual tree to a new directory target.
type Writer interface {
	Exists(path string) (bool, error)
	MaterializeNewDir(target string, tree *artifact.Tree) error
}

// BundleWriter writes a verified bundle archive atomically.
type BundleWriter interface {
	Exists(path string) (bool, error)
	WriteBundleAtomic(path string, data []byte) error
}

// Options configures a directory or bundle build.
type Options struct {
	Policy      policy.CompilePolicy
	Set         map[string]string
	Values      map[string]any
	Workspace   inputs.FS
	LockPath    string
	Output      string
	Format      string // directory|bundle
	EmbedImages bool
	ImageCache  string // verified OCI layout from `images fetch`
	// FrontendRunner executes package-manager builds. Nil uses the PATH runner.
	FrontendRunner frontend.Runner
	// FrontendCache stores non-empty package outputs keyed by source hash.
	FrontendCache string
}

// Result is the build outcome.
type Result struct {
	Diagnostics  []diagnostic.Diagnostic
	OK           bool
	Output       string
	TreePaths    []string
	Preview      bool
	Format       string
	Tree         *artifact.Tree // set for successful compile; used by bundle packing
	Deployment   ir.Deployment
	BundleSHA256 string // set for successful bundle builds
}

// Run validates against lock, compiles IR/graph, builds the virtual tree, and materializes it.
func Run(fs validate.FileReader, writer Writer, manifestPath string, opts Options) Result {
	return RunWithBundle(fs, writer, nil, manifestPath, opts)
}

// RunWithBundle supports directory and bundle formats.
func RunWithBundle(fs validate.FileReader, writer Writer, bwriter BundleWriter, manifestPath string, opts Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	if opts.Format == "" {
		opts.Format = "directory"
	}
	res := Result{Format: opts.Format}
	if opts.Format != "directory" && opts.Format != "bundle" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			fmt.Sprintf("unsupported --format %q (allowed: directory, bundle)", opts.Format))}
		return finish(res)
	}
	if opts.Output == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "build requires --output <path>")}
		return finish(res)
	}

	lockPath := opts.LockPath
	if lockPath == "" {
		lockPath = filepath.Join(filepath.Dir(manifestPath), "iacpack.lock.json")
	}
	lockRaw, err := fs.ReadFile(lockPath)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse,
			"failed to read lockfile: "+err.Error()).WithSource(lockPath, "", 0, 0).
			WithHint("Run iacpack lock before build"))
		return finish(res)
	}
	locked, err := lockfile.Parse(lockRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse, err.Error()).WithSource(lockPath, "", 0, 0))
		return finish(res)
	}

	vres := validate.RunWithOptions(fs, manifestPath, validate.Options{
		Policy:    opts.Policy,
		Set:       opts.Set,
		Values:    opts.Values,
		Workspace: opts.Workspace,
		Locked:    true,
		LockPath:  lockPath,
	})
	res.Diagnostics = append(res.Diagnostics, vres.Diagnostics...)
	if !vres.OK {
		return finish(res)
	}

	pkgFormat := opts.Format
	if pkgFormat == "bundle" {
		pkgFormat = "bundle"
	}
	res.Diagnostics = append(res.Diagnostics, policy.PackageFormatConflict(vres.Project.Spec.Package, pkgFormat)...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}

	dep, err := compile.ToIR(vres.Project)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "IR: "+err.Error()))
		return finish(res)
	}
	g, gdiags := compile.BuildGraph(dep)
	res.Diagnostics = append(res.Diagnostics, gdiags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}

	frontendCache := opts.FrontendCache
	if frontendCache == "" && len(vres.Project.Spec.Builds) > 0 {
		frontendCache = filepath.Join(state.ResolveHome(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME")), "spa-cache")
	}
	tree, preview, tdiags := assembleTree(assembleOpts{
		Project:       vres.Project,
		Deployment:    dep,
		Graph:         g,
		Locked:        locked,
		LockRaw:       lockRaw,
		Policy:        opts.Policy,
		Set:           opts.Set,
		Values:        opts.Values,
		Workspace:     opts.Workspace,
		Frontend:      opts.FrontendRunner,
		FrontendCache: frontendCache,
	})
	res.Diagnostics = append(res.Diagnostics, tdiags...)
	res.Preview = preview
	res.Tree = tree
	res.Deployment = dep
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	if opts.EmbedImages {
		res.Diagnostics = append(res.Diagnostics, embedImages(tree, locked, opts.ImageCache)...)
		if diagnostic.HasError(res.Diagnostics) {
			return finish(res)
		}
		dep.Package.EmbedImages = true
		res.Deployment = dep
	}

	if opts.Format == "bundle" {
		if bwriter == nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "bundle writer required"))
			return finish(res)
		}
		raw, err := bundle.Pack(bundle.PackOptions{
			Deployment: dep,
			Policy:     opts.Policy,
			Tree:       tree,
			HasPreview: preview,
		})
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "bundle pack: "+err.Error()))
			return finish(res)
		}
		if err := bwriter.WriteBundleAtomic(opts.Output, raw); err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
				"failed to write bundle: "+err.Error()))
			return finish(res)
		}
		sum := sha256.Sum256(raw)
		res.Output = opts.Output
		res.TreePaths = tree.Paths()
		res.BundleSHA256 = hex.EncodeToString(sum[:])
		res.OK = true
		return finish(res)
	}

	exists, err := writer.Exists(opts.Output)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	if exists {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
			fmt.Sprintf("refusing to overwrite existing directory target %s", opts.Output)).
			WithHint("Choose a new --output path; directory replace is unsupported in v1"))
		return finish(res)
	}
	if err := writer.MaterializeNewDir(opts.Output, tree); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
			"failed to materialize build output: "+err.Error()))
		return finish(res)
	}
	res.Output = opts.Output
	res.TreePaths = tree.Paths()
	res.OK = true
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}

type assembleOpts struct {
	Project       domain.Project
	Deployment    ir.Deployment
	Graph         plan.Graph
	Locked        lockfile.Document
	LockRaw       []byte
	Policy        policy.CompilePolicy
	Set           map[string]string
	Values        map[string]any
	Workspace     inputs.FS
	Frontend      frontend.Runner
	FrontendCache string
}

func assembleTree(o assembleOpts) (*artifact.Tree, bool, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	tree := artifact.New()

	canon, err := v1alpha1.CanonicalSemantic(o.Project)
	if err != nil {
		return nil, false, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, err.Error())}
	}
	if err := tree.AddFile("manifest.canonical.json", artifact.ModeFile, append(canon, '\n')); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
	}

	depBytes, err := compile.MarshalDeploymentCanonical(o.Deployment)
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, "deployment.json: "+err.Error()))
	} else if err := tree.AddFile("deployment.json", artifact.ModeFile, depBytes); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
	}

	if err := tree.AddFile("iacpack.lock.json", artifact.ModeFile, normalizeNL(o.LockRaw)); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
	}

	polBytes, err := policy.CanonicalBytes(o.Policy)
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, "policy: "+err.Error()))
	} else {
		sum := sha256.Sum256(polBytes)
		hexSum := hex.EncodeToString(sum[:])
		if o.Locked.Policy.SHA256 != "" && o.Locked.Policy.SHA256 != hexSum {
			diags = append(diags, diagnostic.Error(lockfile.CodeLockDrift,
				"compile policy digest does not match lockfile"))
		}
		if err := tree.AddFile("policy.compile.json", artifact.ModeFile, append(polBytes, '\n')); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	}

	diags = append(diags, copyLockedInputs(tree, o.Project, o.Locked, o.Workspace)...)
	if len(o.Project.Spec.Builds) > 0 {
		files, bdiags := frontend.Materialize(o.Workspace, o.Project.Spec.Builds, o.Frontend, o.FrontendCache)
		diags = append(diags, bdiags...)
		for _, id := range sortedBuildIDs(files) {
			for _, f := range files[id] {
				mode := artifact.ModeFile
				if f.Mode&0o111 != 0 {
					mode = artifact.ModeExec
				}
				dest := path.Join("payload", "build-"+id, f.Rel)
				if err := tree.AddFile(dest, mode, f.Body); err != nil {
					diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend, err.Error()))
				}
			}
		}
	}

	deployResolved, deployDiags := params.ResolveDeploy(o.Project, params.Options{Set: o.Set, Values: o.Values})
	missing := missingParamIDs(deployDiags)
	// Missing deploy defaults are not fatal for directory build; they suppress preview.
	for _, d := range deployDiags {
		if d.Code != params.CodeParamMissing {
			diags = append(diags, d)
		}
	}

	can, need := compose.CanPreview(o.Deployment, deployResolved)
	if len(missing) > 0 {
		can = false
		need = missing
	}
	if can && !diagnostic.HasError(diags) {
		deployVals := map[string]string{}
		for id, r := range deployResolved {
			deployVals[id] = scalarString(r)
		}
		cf, err := compose.Render(o.Deployment, compose.RenderOptions{DeployValues: deployVals, Lock: o.Locked})
		if err != nil {
			diags = append(diags, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		} else if raw, err := compose.MarshalYAML(cf); err != nil {
			diags = append(diags, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		} else if err := tree.AddFile("rendered/compose.preview.yaml", artifact.ModeFile, raw); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	} else {
		rec := map[string]any{
			"reason":     "DEPLOY_PARAMETER_DEFAULT_MISSING",
			"parameters": need,
			"message":    "compose.preview.yaml omitted because one or more deploy parameters lack resolvable defaults",
		}
		raw, _ := canonicaljson.Marshal(rec)
		_ = tree.AddFile("rendered/preview-unavailable.json", artifact.ModeFile, append(raw, '\n'))
	}

	genReadme := true
	genEnv := true
	if o.Project.Spec.Package.Set {
		genReadme = o.Project.Spec.Package.GenerateReadme
		genEnv = o.Project.Spec.Package.GenerateEnvExample
	}
	if genReadme {
		if err := tree.AddFile("docs/README.md", artifact.ModeFile, []byte(docs.README(o.Deployment, o.Graph))); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	}
	if err := tree.AddFile("docs/architecture.mmd", artifact.ModeFile, []byte(compile.FormatGraphMermaid(o.Graph))); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
	}
	if genEnv {
		if err := tree.AddFile("config/.env.example", artifact.ModeFile, []byte(docs.EnvExample(o.Deployment))); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	}
	if _, ok := tree.Get("config/.env"); ok {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, "invariant: .env must not be packaged"))
	}
	return tree, can, diags
}

func missingParamIDs(diags []diagnostic.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		if d.Code != params.CodeParamMissing {
			continue
		}
		// message: `build parameter %q has no value...` or similar — extract via source pointer
		if d.Source != nil && strings.HasPrefix(d.Source.Pointer, "/spec/parameters/") {
			out = append(out, strings.TrimPrefix(d.Source.Pointer, "/spec/parameters/"))
		}
	}
	return out
}

func sortedBuildIDs(files map[string][]frontend.File) []string {
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func copyLockedInputs(tree *artifact.Tree, project domain.Project, locked lockfile.Document, ws inputs.FS) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	if ws == nil {
		if len(locked.Inputs) > 0 {
			return []diagnostic.Diagnostic{diagnostic.Error(inputs.CodeInputIO, "workspace required to copy locked inputs")}
		}
		return nil
	}
	for _, in := range locked.Inputs {
		data, err := ws.ReadFile(in.Path)
		if err != nil {
			diags = append(diags, diagnostic.Error(inputs.CodeInputIO, fmt.Sprintf("read %q: %v", in.Path, err)))
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != in.SHA256 {
			diags = append(diags, diagnostic.Error(lockfile.CodeLockDrift,
				fmt.Sprintf("input %q hash drifted during build copy", in.Path)))
			continue
		}
		if int64(len(data)) != in.Size {
			diags = append(diags, diagnostic.Error(inputs.CodeInputChanged,
				fmt.Sprintf("input %q size changed during build copy", in.Path)))
			continue
		}
		dest, err := artifactDest(project, in)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
			continue
		}
		mode := artifact.FileMode(in.Mode)
		if mode == "" {
			mode = artifact.ModeFile
		}
		if err := tree.AddFile(dest, mode, data); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	}
	diags = append(diags, addDatabaseScripts(tree, project)...)
	return diags
}

func artifactDest(project domain.Project, in lockfile.InputLock) (string, error) {
	src := in.Source
	if strings.HasPrefix(src, "resource/") {
		id := strings.TrimPrefix(src, "resource/")
		r, ok := project.Spec.Resources[id]
		if !ok {
			return "", fmt.Errorf("unknown resource source %q", src)
		}
		rel := in.Path
		base := r.Source.String()
		if base != "." && strings.HasPrefix(rel, base+"/") {
			rel = strings.TrimPrefix(rel, base+"/")
		}
		return path.Join("payload", id, rel), nil
	}
	if strings.HasPrefix(src, "database/") {
		id := strings.TrimPrefix(src, "database/")
		db, ok := project.Spec.Databases[id]
		if !ok {
			return "", fmt.Errorf("unknown database source %q", src)
		}
		rel := in.Path
		base := db.Migrations
		if base != "." && strings.HasPrefix(rel, base+"/") {
			rel = strings.TrimPrefix(rel, base+"/")
		}
		return path.Join("payload", "db-"+id, "sql", rel), nil
	}
	if strings.HasPrefix(src, "image/") && strings.HasSuffix(src, "/build") {
		mid := strings.TrimSuffix(strings.TrimPrefix(src, "image/"), "/build")
		img := project.Spec.Images[mid]
		if img.Build == nil {
			return "", fmt.Errorf("build input for non-build image %q", mid)
		}
		ctx := img.Build.Context.String()
		rel := in.Path
		if ctx != "." && strings.HasPrefix(rel, ctx+"/") {
			rel = strings.TrimPrefix(rel, ctx+"/")
		}
		return path.Join("builds", mid, rel), nil
	}
	return "", fmt.Errorf("unsupported lock input source %q", src)
}

func scalarString(r params.Resolved) string {
	switch r.Value.Kind {
	case domain.ScalarString:
		return r.Value.Str
	case domain.ScalarBool:
		if r.Value.Bool {
			return "true"
		}
		return "false"
	case domain.ScalarNumber:
		return r.Value.Str
	default:
		return ""
	}
}

func embedImages(tree *artifact.Tree, locked lockfile.Document, cacheDir string) []diagnostic.Diagnostic {
	if cacheDir == "" {
		return []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"--embed-images requires --image-cache <dir>")}
	}
	var expected []string
	for _, img := range locked.Images {
		if img.Build {
			continue
		}
		d := img.Digest
		if d == "" {
			if dd, ok := oci.DigestOf(img.Ref); ok {
				d = dd
			}
		}
		if d == "" {
			return []diagnostic.Diagnostic{diagnostic.Error(lockfile.CodeLockImageUnresolved,
				"cannot embed image without digest")}
		}
		expected = append(expected, d)
	}
	cache := &layout.Cache{Root: cacheDir}
	if err := cache.Verify(expected); err != nil {
		return []diagnostic.Diagnostic{diagnostic.Error("IMAGES.VERIFY",
			"image-cache verification failed: "+err.Error()).
			WithHint("Run iacpack images fetch before build --embed-images")}
	}
	var diags []diagnostic.Diagnostic
	err := filepath.Walk(cacheDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(cacheDir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return tree.AddFile(path.Join("images/oci-layout", filepath.ToSlash(rel)), artifact.ModeFile, data)
	})
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
	}
	return diags
}

func normalizeNL(b []byte) []byte {
	if len(b) == 0 {
		return []byte{'\n'}
	}
	if b[len(b)-1] != '\n' {
		return append(append([]byte(nil), b...), '\n')
	}
	return append([]byte(nil), b...)
}

// OSWriter materializes trees using the local OS (staging + rename).
type OSWriter struct{}

func (OSWriter) Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// WriteBundleAtomic stages bytes, re-opens and verifies the archive, then
// atomically renames onto path (creating or replacing). A failed verify leaves
// any previous successful output untouched.
func (OSWriter) WriteBundleAtomic(path string, data []byte) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging := abs + ".iacpack-staging"
	_ = os.Remove(staging)
	f, err := os.OpenFile(staging, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(staging)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	rf, err := os.Open(staging)
	if err != nil {
		return err
	}
	vres, diags := bundle.Verify(rf, bundle.VerifyOptions{Quotas: bundle.DefaultQuotas()})
	_ = rf.Close()
	if len(diags) > 0 || vres.Meta.FormatVersion == "" {
		return fmt.Errorf("post-write bundle verification failed")
	}
	if err := os.Rename(staging, abs); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (OSWriter) MaterializeNewDir(target string, tree *artifact.Tree) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("target exists: %s", abs)
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging := abs + ".iacpack-staging"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	for _, e := range tree.Sorted() {
		full := filepath.Join(staging, filepath.FromSlash(e.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if e.Mode == artifact.ModeExec {
			perm = 0o755
		}
		if err := os.WriteFile(full, e.Content, perm); err != nil {
			return err
		}
		sum := sha256.Sum256(e.Content)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			return fmt.Errorf("internal hash mismatch for %s", e.Path)
		}
	}
	if err := os.Rename(staging, abs); err != nil {
		return err
	}
	cleanup = false
	return nil
}
