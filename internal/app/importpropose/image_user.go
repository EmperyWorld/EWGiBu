package importpropose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"iacpack/internal/diagnostic"
)

// imageConfigUser reads Config.User from a local engine image. The bool is
// false when no local engine can inspect the ref (image not present). An
// empty user with true means the image has no USER. No network pull.
var imageConfigUser = imageConfigUserHost

func imageConfigUserHost(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, bin := range []string{"podman", "docker"} {
		cmd := exec.CommandContext(ctx, bin, "image", "inspect", "--format", "{{.Config.User}}", ref)
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(out)), true
	}
	return "", false
}

func numericImportUser(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return false
		}
		return digitsOnly(parts[0]) && digitsOnly(parts[1])
	}
	return digitsOnly(s)
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// applyImageUsers sets service.user from image Config.User when compose omitted
// user. Numeric uid/uid:gid are written. Names become IMPORT.USER_NAME.
// Existing draft user values are kept.
func applyImageUsers(services, images map[string]any, workspace string, existing map[string]any, diags *[]diagnostic.Diagnostic) {
	existingUsers := map[string]string{}
	if existing != nil {
		if spec, _ := existing["spec"].(map[string]any); spec != nil {
			if svcs, _ := spec["services"].(map[string]any); svcs != nil {
				for id, raw := range svcs {
					body, _ := raw.(map[string]any)
					if body == nil {
						continue
					}
					if u, ok := body["user"].(string); ok && u != "" {
						existingUsers[id] = u
					}
				}
			}
		}
	}
	ids := make([]string, 0, len(services))
	for id := range services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		body, _ := services[id].(map[string]any)
		if body == nil {
			continue
		}
		if keep, ok := existingUsers[id]; ok {
			body["user"] = keep
			services[id] = body
			continue
		}
		if u, ok := body["user"].(string); ok && u != "" {
			continue
		}
		imgID, _ := body["image"].(string)
		img, _ := images[imgID].(map[string]any)
		if img == nil {
			continue
		}
		ref, _ := img["ref"].(string)
		if ref == "" {
			continue
		}
		cfg, found := imageConfigUser(ref)
		if !found {
			if du, ok := dockerfileUser(workspace, img); ok {
				noteDockerfileUser(id, du, body, services, diags)
				continue
			}
			*diags = append(*diags, diagnostic.Warning(diagnostic.CodeImportUserAbsent,
				fmt.Sprintf("service %s: image %q is not local; Config.User was not read", id, ref)).
				WithSource("", "/spec/services/"+id+"/user", 0, 0).
				WithHint("Load the image, or set USER in the Dockerfile, then import again"))
			continue
		}
		if cfg == "" {
			if du, ok := dockerfileUser(workspace, img); ok {
				noteDockerfileUser(id, du, body, services, diags)
			}
			continue
		}
		if numericImportUser(cfg) {
			body["user"] = cfg
			services[id] = body
			continue
		}
		*diags = append(*diags, diagnostic.Info(diagnostic.CodeImportUserName,
			fmt.Sprintf("service %s: image Config.User %q is a name; set service.user to a numeric uid for rootless chown", id, cfg)).
			WithSource("", "/spec/services/"+id+"/user", 0, 0))
	}
}

func noteDockerfileUser(id, du string, body map[string]any, services map[string]any, diags *[]diagnostic.Diagnostic) {
	if numericImportUser(du) {
		body["user"] = du
		services[id] = body
		return
	}
	*diags = append(*diags, diagnostic.Info(diagnostic.CodeImportUserName,
		fmt.Sprintf("service %s: Dockerfile USER %q is a name; set service.user to a numeric uid for rootless chown", id, du)).
		WithSource("", "/spec/services/"+id+"/user", 0, 0))
}

func dockerfileUser(workspace string, img map[string]any) (string, bool) {
	build, _ := img["build"].(map[string]any)
	if build == nil || workspace == "" {
		return "", false
	}
	ctx, _ := build["context"].(string)
	df, _ := build["dockerfile"].(string)
	if df == "" {
		df = "Dockerfile"
	}
	rel := df
	if ctx != "" && ctx != "." {
		rel = strings.TrimSuffix(ctx, "/") + "/" + df
	}
	if strings.Contains(rel, "..") {
		return "", false
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	raw, err := os.ReadFile(abs)
	if err != nil {
		return "", false
	}
	var last string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "USER") {
			continue
		}
		user := strings.Trim(fields[1], `"'`)
		if def, ok := composeSingleInterpDefault(user); ok {
			user = def
		}
		last = user
	}
	if last == "" {
		return "", false
	}
	return last, true
}
