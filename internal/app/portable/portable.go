// Package portable writes a transfer kit: lock, non-secret values, secret
// names, image pull/local-only marks, and host paths. It does not write
// secret values.
package portable

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/params"
)

const (
	codeLocalImage = "PORTABLE.LOCAL_IMAGE"
	codeHostPath   = "PORTABLE.HOST_PATH"
)

// Options selects the workspace and the directory that receives the kit.
type Options struct {
	Workspace        string
	OutputDir        string
	EmbedLocalImages bool
}

// Result is the kit outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run writes the portable kit. An empty host-path template and a local-only
// image that was not embedded are warnings: the files are the handoff, and
// apply is the gate that refuses to start containers. A failed image save
// stays an error.
func Run(opts Options) Result {
	var res Result
	if opts.Workspace == "" || opts.OutputDir == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "bundle requires --workspace and --output-dir")}
		return finish(res)
	}
	ws, err := filepath.Abs(opts.Workspace)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	manifest := filepath.Join(ws, "iacpack.yaml")
	lockPath := filepath.Join(ws, "iacpack.lock.json")
	raw, err := os.ReadFile(manifest)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "workspace manifest: "+err.Error())}
		return finish(res)
	}
	lockRaw, err := os.ReadFile(lockPath)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "workspace lock: "+err.Error()).
			WithHint("iacpack lock --file " + manifest)}
		return finish(res)
	}
	proj, diags := v1alpha1.Decode(loader.Parse(manifest, raw))
	res.Diagnostics = append(res.Diagnostics, diags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	out, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	if err := os.WriteFile(filepath.Join(out, "iacpack.lock.json"), lockRaw, 0o644); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	values, omitted := valuesWithoutSecrets(proj)
	valRaw, err := canonicaljson.Marshal(values)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, err.Error())}
		return finish(res)
	}
	valRaw = append(valRaw, '\n')
	if err := os.WriteFile(filepath.Join(out, "values.json"), valRaw, 0o644); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	secrets := requiredSecrets(proj)
	if err := os.WriteFile(filepath.Join(out, "secrets.txt"), []byte(strings.Join(secrets, "\n")+"\n"), 0o644); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	images := imageLines(proj)
	if opts.EmbedLocalImages {
		var next []string
		for _, line := range images {
			ref, class, _ := strings.Cut(line, "\t")
			if class != "local-only" {
				next = append(next, line)
				continue
			}
			rel, err := saveLocalImage(out, ref)
			if err != nil {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(codeLocalImage, err.Error()))
				next = append(next, line)
				continue
			}
			next = append(next, ref+"\tload\t"+rel)
		}
		images = next
	}
	if err := os.WriteFile(filepath.Join(out, "images.txt"), []byte(strings.Join(images, "\n")+"\n"), 0o644); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	hosts := hostPathTemplate(proj)
	hostRaw, err := canonicaljson.Marshal(hosts)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, err.Error())}
		return finish(res)
	}
	hostRaw = append(hostRaw, '\n')
	if err := os.WriteFile(filepath.Join(out, "host-paths.json"), hostRaw, 0o644); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	for _, line := range images {
		ref, class, _ := strings.Cut(line, "\t")
		if class == "local-only" {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Warning(codeLocalImage,
				fmt.Sprintf("image %s is local-only", ref)).
				WithHint("Pass --embed-local-images, or pin a registry digest before moving this kit"))
		}
	}
	for _, raw := range hosts {
		h, _ := raw.(map[string]any)
		if val, _ := h["value"].(string); strings.TrimSpace(val) != "" {
			continue
		}
		param, _ := h["parameter"].(string)
		if param == "" {
			param, _ = h["literal"].(string)
		}
		res.Diagnostics = append(res.Diagnostics, diagnostic.Warning(codeHostPath,
			fmt.Sprintf("service %s mount %s host path template %s", h["service"], h["target"], param)).
			WithHint("Fill value in host-paths.json on the destination machine; apply refuses an empty value"))
	}
	for _, name := range omitted {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Warning(params.CodeParamSecretLike,
			fmt.Sprintf("omitted secret-like parameter %s from values.json", name)))
	}
	res.Model = map[string]any{
		"outputDir": out,
		"secrets":   secrets,
		"images":    images,
		"hostPaths": hosts,
	}
	res.Text = formatText(out, secrets, images, hosts)
	res.OK = !diagnostic.HasError(res.Diagnostics)
	return finish(res)
}

func valuesWithoutSecrets(p domain.Project) (map[string]any, []string) {
	out := map[string]any{}
	var omitted []string
	ids := make([]string, 0, len(p.Spec.Parameters))
	for id := range p.Spec.Parameters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		param := p.Spec.Parameters[id]
		if param.Phase != domain.ParameterPhaseDeploy || param.Default == nil {
			continue
		}
		if params.SecretLikeName(id) {
			omitted = append(omitted, id)
			continue
		}
		out[id] = scalarJSON(*param.Default)
	}
	return out, omitted
}

func scalarJSON(s domain.Scalar) any {
	switch s.Kind {
	case domain.ScalarBool:
		return s.Bool
	case domain.ScalarNumber:
		return s.Num
	default:
		return s.Str
	}
}

func requiredSecrets(p domain.Project) []string {
	var ids []string
	for id, sec := range p.Spec.Secrets {
		if sec.Required {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if ids == nil {
		return []string{}
	}
	return ids
}

func imageLines(p domain.Project) []string {
	ids := make([]string, 0, len(p.Spec.Images))
	for id := range p.Spec.Images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var lines []string
	for _, id := range ids {
		img := p.Spec.Images[id]
		ref := img.Ref
		class := "pull"
		if ref == "" && img.RefParameter != "" {
			ref = "parameter:" + img.RefParameter
		} else if img.Build != nil || localOnlyRef(ref) {
			class = "local-only"
		}
		if ref == "" && img.Build != nil {
			ref = "build:" + id
			class = "local-only"
		}
		lines = append(lines, ref+"\t"+class)
	}
	return lines
}

func localOnlyRef(ref string) bool {
	low := strings.ToLower(ref)
	if strings.HasPrefix(low, "localhost/") || strings.HasPrefix(low, "localhost:") || strings.HasPrefix(low, "127.0.0.1/") {
		return true
	}
	if !strings.Contains(ref, "@sha256:") {
		return true
	}
	return false
}

func hostPathTemplate(p domain.Project) []any {
	type row struct {
		service string
		item    map[string]any
	}
	var rows []row
	add := func(service string, mounts []domain.Mount) {
		for _, m := range mounts {
			if m.HostLiteral == "" && m.HostParameter == "" {
				continue
			}
			item := map[string]any{"service": service, "target": m.Target.String(), "value": ""}
			if m.HostParameter != "" {
				item["parameter"] = m.HostParameter
			} else {
				// Absolute host path becomes a fillable template on the other machine.
				item["parameter"] = hostParamID(service, m.Target.String())
				item["literal"] = m.HostLiteral
			}
			rows = append(rows, row{service: service + "\x00" + m.Target.String(), item: item})
		}
	}
	sids := make([]string, 0, len(p.Spec.Services))
	for id := range p.Spec.Services {
		sids = append(sids, id)
	}
	sort.Strings(sids)
	for _, id := range sids {
		add(id, p.Spec.Services[id].Mounts)
	}
	tids := make([]string, 0, len(p.Spec.Tasks))
	for id := range p.Spec.Tasks {
		tids = append(tids, id)
	}
	sort.Strings(tids)
	for _, id := range tids {
		add(id, p.Spec.Tasks[id].Mounts)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].service < rows[j].service })
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r.item
	}
	return out
}

func formatText(dir string, secrets, images []string, hosts []any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kit: %s\n", dir)
	fmt.Fprintf(&b, "secrets: %s\n", strings.Join(secrets, ", "))
	for _, line := range images {
		ref, class, _ := strings.Cut(line, "\t")
		fmt.Fprintf(&b, "image %s %s\n", class, ref)
	}
	for _, raw := range hosts {
		h, _ := raw.(map[string]any)
		param, _ := h["parameter"].(string)
		fmt.Fprintf(&b, "host %s %s parameter %s value=%q\n", h["service"], h["target"], param, h["value"])
	}
	return b.String()
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
