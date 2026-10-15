// Package frontend checks and runs package-manager builds for iacpack build.
// validate only checks package.json and the lockfile. The package manager runs
// during iacpack build, outside the source workspace, and the output is copied
// into the bundle payload. apply mounts that payload and does not build.
package frontend

import (
	"encoding/json"
	"path"
	"strings"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/inputs"
)

// LockfileName is the frozen lockfile required for a package manager.
func LockfileName(manager string) string {
	switch manager {
	case "pnpm":
		return "pnpm-lock.yaml"
	case "npm":
		return "package-lock.json"
	case "yarn":
		return "yarn.lock"
	case "bun":
		return "bun.lock"
	default:
		return ""
	}
}

// Check reports missing package.json, lockfile, or script. It does not run the manager.
func Check(ws inputs.FS, builds map[string]domain.PackageBuild) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	if ws == nil {
		if len(builds) > 0 {
			diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
				"workspace is required to validate package builds"))
		}
		return diags
	}
	for id, b := range builds {
		ctx := strings.Trim(b.Context.String(), "/")
		pkgPath := path.Join(ctx, "package.json")
		raw, err := ws.ReadFile(pkgPath)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
				"package build "+id+": package.json not found at "+pkgPath))
			continue
		}
		var doc struct {
			Scripts map[string]any `json:"scripts"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
				"package build "+id+": package.json is not JSON"))
			continue
		}
		if _, ok := doc.Scripts[b.Script]; !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
				"package build "+id+": package.json has no script "+b.Script))
		}
		if b.Install == "frozen" {
			lock := LockfileName(b.PackageManager)
			if lock == "" {
				diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
					"package build "+id+": unknown package manager "+b.PackageManager))
				continue
			}
			lockPath := path.Join(ctx, lock)
			if _, err := ws.Lstat(lockPath); err != nil && b.PackageManager == "bun" {
				if _, err2 := ws.Lstat(path.Join(ctx, "bun.lockb")); err2 != nil {
					diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
						"package build "+id+": frozen install requires "+lock+" or bun.lockb"))
				}
				continue
			}
			if err != nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeBuildFrontend,
					"package build "+id+": frozen install requires "+lockPath))
				continue
			}
		}
		if b.PackageManager == "pnpm" {
			_, extra := outsideRoots(ws, id, ctx)
			diags = append(diags, extra...)
		}
		_, _, srcDiags := sourceOutside(ws, id, ctx)
		diags = append(diags, srcDiags...)
	}
	return diags
}
