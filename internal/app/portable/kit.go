package portable

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"iacpack/internal/diagnostic"
)

const (
	CodeLocalImage = "PORTABLE.LOCAL_IMAGE"
	CodeHostPath   = "PORTABLE.HOST_PATH"
)

// CheckKit stops apply before containers when a portable kit still has empty
// host paths or local-only images that are not present in the engine.
func CheckKit(kitDir string) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	hostsPath := filepath.Join(kitDir, "host-paths.json")
	if raw, err := os.ReadFile(hostsPath); err == nil {
		var rows []map[string]any
		if err := json.Unmarshal(raw, &rows); err == nil {
			var empty []string
			for _, row := range rows {
				val, _ := row["value"].(string)
				param, _ := row["parameter"].(string)
				svc, _ := row["service"].(string)
				tgt, _ := row["target"].(string)
				if absoluteHostPath(val) {
					continue
				}
				label := svc + ":" + tgt
				if param != "" {
					label = param
				}
				empty = append(empty, label)
			}
			sort.Strings(empty)
			if len(empty) > 0 {
				diags = append(diags, diagnostic.Error(CodeHostPath,
					"host paths not set: "+strings.Join(empty, ", ")).
					WithHint("Write each value in "+hostsPath+" then re-run apply"))
			}
		}
	}
	imagesPath := filepath.Join(kitDir, "images.txt")
	if raw, err := os.ReadFile(imagesPath); err == nil {
		var missing []string
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			ref := parts[0]
			class := ""
			if len(parts) > 1 {
				class = parts[1]
			}
			switch class {
			case "local-only", "load":
				if !imagePresent(ref) {
					missing = append(missing, ref)
				}
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			diags = append(diags, diagnostic.Error(CodeLocalImage,
				"local images not loaded: "+strings.Join(missing, ", ")).
				WithHint("podman load -i <kit>/images/<name>.tar (or docker load), then re-run apply"))
		}
	}
	return diags
}

// KitDeployValues reads filled host-path parameter values from a kit directory.
func KitDeployValues(kitDir string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(kitDir, "host-paths.json"))
	if err != nil {
		return out
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return out
	}
	for _, row := range rows {
		param, _ := row["parameter"].(string)
		val, _ := row["value"].(string)
		if param != "" && strings.TrimSpace(val) != "" {
			out[param] = val
		}
	}
	return out
}

// IsKitDir reports a portable transfer directory (not a .iacbundle).
func IsKitDir(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, "images.txt")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, "host-paths.json")); err != nil {
		return false
	}
	return true
}

// FindBundleInKit returns the first *.iacbundle under the kit, if any.
func FindBundleInKit(kitDir string) string {
	entries, err := os.ReadDir(kitDir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(strings.ToLower(n), ".iacbundle") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return filepath.Join(kitDir, names[0])
}

// imagePresent reports whether a local engine already has ref. Tests replace it.
var imagePresent = engineHasImage

func engineHasImage(ref string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, bin := range []string{"podman", "docker"} {
		cmd := exec.CommandContext(ctx, bin, "image", "inspect", ref)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func saveLocalImage(outDir, ref string) (rel string, err error) {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, ref)
	if safe == "" {
		safe = "image"
	}
	rel = filepath.ToSlash(filepath.Join("images", safe+".tar"))
	abs := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var last error
	for _, bin := range []string{"podman", "docker"} {
		cmd := exec.CommandContext(ctx, bin, "save", "-o", abs, ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			last = fmt.Errorf("%s save %s: %s", bin, ref, strings.TrimSpace(string(out)))
			continue
		}
		return rel, nil
	}
	if last == nil {
		last = fmt.Errorf("no container engine to save %s", ref)
	}
	return "", last
}
