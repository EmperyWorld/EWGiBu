// Package lock implements the iacpack lock use-case.
package lock

import (
	"os"
	"path/filepath"

	"iacpack/internal/app/validate"
	"iacpack/internal/diagnostic"
	"iacpack/internal/inputs"
	"iacpack/internal/lockfile"
	"iacpack/internal/oci"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

// FileSystem is the lock command's FS surface.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFileAtomicReplace(path string, data []byte, perm os.FileMode) error
}

// Options configures lock generation.
type Options struct {
	Policy       policy.CompilePolicy
	Set          map[string]string
	Values       map[string]any
	Workspace    inputs.FS
	LockPath     string
	UpdateImages bool
	Resolver     oci.Resolver
}

// Result is the lock command outcome.
type Result struct {
	Document    lockfile.Document
	Bytes       []byte
	LockPath    string
	Diagnostics []diagnostic.Diagnostic
	OK          bool
	BuildParams map[string]params.Resolved
	Inputs      []inputs.FileRecord
}

// Run validates fully, builds the lock document, and atomically writes it.
func Run(fs FileSystem, manifestPath string, opts Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	vres := validate.RunWithOptions(fs, manifestPath, validate.Options{
		Policy:       opts.Policy,
		Set:          opts.Set,
		Values:       opts.Values,
		Workspace:    opts.Workspace,
		UpdateImages: opts.UpdateImages,
		Resolver:     opts.Resolver,
	})
	res := Result{
		Diagnostics: append([]diagnostic.Diagnostic{}, vres.Diagnostics...),
		BuildParams: vres.BuildParams,
		Inputs:      vres.Inputs,
		Document:    vres.LockDocument,
	}
	if !vres.OK {
		res.OK = false
		diagnostic.SortDeterministic(res.Diagnostics)
		return res
	}
	lockPath := opts.LockPath
	if lockPath == "" {
		lockPath = filepath.Join(filepath.Dir(manifestPath), "iacpack.lock.json")
	}
	res.LockPath = lockPath
	raw, err := lockfile.MarshalCanonical(vres.LockDocument)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "marshal lockfile: "+err.Error()))
		res.OK = false
		return res
	}
	res.Bytes = raw
	if err := fs.WriteFileAtomicReplace(lockPath, raw, 0o644); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
			"failed to write lockfile: "+err.Error()).WithSource(lockPath, "", 0, 0))
		res.OK = false
		diagnostic.SortDeterministic(res.Diagnostics)
		return res
	}
	res.OK = true
	diagnostic.SortDeterministic(res.Diagnostics)
	return res
}
