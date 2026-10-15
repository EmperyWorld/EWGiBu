package plan

import (
	"os"
	"path/filepath"

	"iacpack/internal/diagnostic"
	"iacpack/internal/lockfile"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

// staleLockDiagnostic stops plan/apply when the workspace draft spec does not
// match the lock inside the bundle that would be applied.
func staleLockDiagnostic(workspace, bundlePath, project string, locked lockfile.Document) []diagnostic.Diagnostic {
	dir, explicit := workspaceDir(workspace)
	if dir == "" {
		return nil
	}
	manifest := filepath.Join(dir, "iacpack.yaml")
	raw, err := os.ReadFile(manifest)
	if err != nil {
		if explicit {
			return []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "workspace manifest: "+err.Error())}
		}
		return nil
	}
	proj, diags := v1alpha1.Decode(loader.Parse(manifest, raw))
	if diagnostic.HasError(diags) {
		if explicit {
			return diags
		}
		return nil
	}
	if proj.Metadata.Name != project {
		if explicit {
			return []diagnostic.Diagnostic{diagnostic.Error(lockfile.CodeLockDrift, "lock устарел").
				WithHint("workspace project " + proj.Metadata.Name + " is not " + project)}
		}
		return nil
	}
	hash, err := v1alpha1.SemanticHash(proj)
	if err != nil {
		return []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, err.Error())}
	}
	if hash == locked.Manifest.SemanticHash {
		return nil
	}
	hintTarget := bundlePath
	lockPath := filepath.Join(dir, "iacpack.lock.json")
	if lockRaw, err := os.ReadFile(lockPath); err == nil {
		if doc, err := lockfile.Parse(lockRaw); err == nil && doc.Manifest.SemanticHash != hash {
			hintTarget = lockPath
		}
	}
	return []diagnostic.Diagnostic{diagnostic.Error(lockfile.CodeLockDrift, "lock устарел").
		WithHint("iacpack diff --from " + manifest + " --to " + hintTarget)}
}

func workspaceDir(explicit string) (dir string, explicitSet bool) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return explicit, true
		}
		return abs, true
	}
	if _, err := os.Stat("iacpack.yaml"); err != nil {
		return "", false
	}
	wd, err := os.Getwd()
	if err != nil {
		return ".", false
	}
	return wd, false
}
