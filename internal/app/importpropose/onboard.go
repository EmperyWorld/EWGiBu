package importpropose

import (
	"fmt"
	"sort"
	"strings"
)

// ParameterGap lists an unresolved parameter (no default) for onboarding (ADR-018).
type ParameterGap struct {
	ID     string `json:"id"`
	Phase  string `json:"phase"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// SecretSuggestion is a declaration-only stub suggestion (never carries values).
type SecretSuggestion struct {
	ID          string `json:"id"`
	Source      string `json:"source"` // env_file | env_key | compose_secret
	Path        string `json:"path"`
	Pointer     string `json:"pointer,omitempty"`
	EnvFile     string `json:"envFile,omitempty"`
	EnvKey      string `json:"envKey,omitempty"`
	Service     string `json:"service,omitempty"`
	Target      string `json:"target,omitempty"`
	Description string `json:"description,omitempty"`
}

// ApplyOptions selects which onboarding stubs to merge into a draft Project.
type ApplyOptions struct {
	SecretIDs     []string // suggestion ids to declare under spec.secrets
	Suggestions   []SecretSuggestion
	ParamDefaults map[string]any // non-secret deploy/build defaults only
	AttachMounts  bool           // when true, attach service secret mounts from suggestions
}

// ApplySuggestions merges selected secret declarations (and optional mounts /
// parameter defaults) into project. Never accepts or stores secret values.
func ApplySuggestions(project map[string]any, opt ApplyOptions) (map[string]any, error) {
	if project == nil {
		return nil, fmt.Errorf("project is required")
	}
	out := cloneMap(project)
	spec, _ := out["spec"].(map[string]any)
	if spec == nil {
		spec = map[string]any{}
		out["spec"] = spec
	} else {
		spec = cloneMap(spec)
		out["spec"] = spec
	}

	want := map[string]struct{}{}
	for _, id := range opt.SecretIDs {
		id = sanitizeID(id)
		if id == "" {
			continue
		}
		want[id] = struct{}{}
	}

	secrets, _ := spec["secrets"].(map[string]any)
	if secrets == nil {
		secrets = map[string]any{}
	} else {
		secrets = cloneMap(secrets)
	}

	byID := map[string][]SecretSuggestion{}
	for _, s := range opt.Suggestions {
		id := sanitizeID(s.ID)
		if id == "" {
			continue
		}
		if _, ok := want[id]; !ok {
			continue
		}
		byID[id] = append(byID[id], s)
	}

	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		desc := ""
		for _, s := range byID[id] {
			if s.Description != "" {
				desc = s.Description
				break
			}
		}
		if desc == "" {
			desc = "Imported secret reference (declaration only)"
		}
		secrets[id] = map[string]any{
			"required":    true,
			"delivery":    "file",
			"description": desc,
		}
	}
	if len(secrets) > 0 {
		spec["secrets"] = sortMap(secrets)
	}

	if opt.AttachMounts {
		services, _ := spec["services"].(map[string]any)
		if services != nil {
			services = cloneMap(services)
			for _, id := range ids {
				for _, s := range byID[id] {
					if s.Service == "" || s.Target == "" {
						continue
					}
					svcID := sanitizeID(s.Service)
					svc, _ := services[svcID].(map[string]any)
					if svc == nil {
						continue
					}
					svc = cloneMap(svc)
					mounts, _ := svc["secrets"].([]any)
					if mounts == nil {
						mounts = []any{}
					}
					dup := false
					for _, m := range mounts {
						mm, _ := m.(map[string]any)
						if mm != nil && mm["ref"] == id {
							dup = true
							break
						}
					}
					if !dup {
						mounts = append(mounts, map[string]any{"ref": id, "target": s.Target})
					}
					svc["secrets"] = mounts
					services[svcID] = svc
				}
			}
			spec["services"] = sortMap(services)
		}
	}

	if len(opt.ParamDefaults) > 0 {
		params, _ := spec["parameters"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		} else {
			params = cloneMap(params)
		}
		for id, def := range opt.ParamDefaults {
			id = sanitizeID(id)
			if id == "" || secretNameRe.MatchString(id) {
				continue
			}
			if containsCanary(fmt.Sprint(def)) {
				return nil, fmt.Errorf("parameter default refused: looks like a secret value")
			}
			p, _ := params[id].(map[string]any)
			if p == nil {
				p = map[string]any{"phase": "deploy", "type": "string"}
			} else {
				p = cloneMap(p)
			}
			p["default"] = def
			params[id] = p
		}
		spec["parameters"] = sortMap(params)
		applyImageParameterRefs(spec, params)
	}

	return out, nil
}

// applyImageParameterRefs writes a filled parameter default into image.ref
// when propose stored ref as {parameter: id}.
func applyImageParameterRefs(spec, params map[string]any) {
	images, _ := spec["images"].(map[string]any)
	if len(images) == 0 || len(params) == 0 {
		return
	}
	for id, raw := range images {
		img, _ := raw.(map[string]any)
		if img == nil {
			continue
		}
		refObj, _ := img["ref"].(map[string]any)
		if refObj == nil {
			continue
		}
		pid, _ := refObj["parameter"].(string)
		p, _ := params[pid].(map[string]any)
		if p == nil {
			continue
		}
		def, ok := p["default"]
		if !ok || def == nil || strings.TrimSpace(fmt.Sprint(def)) == "" {
			continue
		}
		next := cloneMap(img)
		next["ref"] = fmt.Sprint(def)
		images[id] = next
	}
	spec["images"] = images
}

// EnvExamplePlaceholders builds `.env.example` text for declared parameters and
// selected secret ids. Placeholders only — never values (ADR-005/018).
func EnvExamplePlaceholders(project map[string]any, secretIDs []string) string {
	var b strings.Builder
	b.WriteString("# Generated by iacpack import onboarding — placeholders only.\n")
	b.WriteString("# Never commit a populated .env. Secret values are never written here.\n")

	wrote := false
	if project != nil {
		spec, _ := project["spec"].(map[string]any)
		params, _ := spec["parameters"].(map[string]any)
		ids := make([]string, 0, len(params))
		for id := range params {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			p, _ := params[id].(map[string]any)
			if p == nil {
				continue
			}
			phase, _ := p["phase"].(string)
			if phase != "" && phase != "deploy" {
				continue
			}
			wrote = true
			key := envKeyFromID(id)
			val := "<set-" + id + ">"
			if def, ok := p["default"]; ok && def != nil {
				val = fmt.Sprint(def)
			}
			if containsCanary(val) {
				val = "<set-" + id + ">"
			}
			if desc, _ := p["description"].(string); desc != "" {
				b.WriteString("# " + strings.ReplaceAll(desc, "\n", " ") + "\n")
			}
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(val)
			b.WriteByte('\n')
		}
	}

	secIDs := append([]string(nil), secretIDs...)
	if project != nil {
		spec, _ := project["spec"].(map[string]any)
		secrets, _ := spec["secrets"].(map[string]any)
		for id := range secrets {
			secIDs = append(secIDs, id)
		}
	}
	seen := map[string]struct{}{}
	var sorted []string
	for _, id := range secIDs {
		id = sanitizeID(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	if len(sorted) > 0 {
		wrote = true
		b.WriteString("\n# Secret references (provide via operator secret files at apply time; not env values):\n")
		for _, id := range sorted {
			b.WriteString("# " + envKeyFromID(id) + "=<operator-secret-file>\n")
		}
	}
	if !wrote {
		b.WriteString("# No deploy parameters or secret stubs declared.\n")
	}
	return b.String()
}

func envKeyFromID(id string) string {
	up := strings.ToUpper(id)
	var b strings.Builder
	for _, r := range up {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func containsCanary(s string) bool {
	upper := strings.ToUpper(s)
	return strings.Contains(upper, "SECRET_CANARY") ||
		strings.Contains(upper, "SUPER_SECRET_CANARY") ||
		strings.Contains(upper, "CANARY_SECRET")
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func buildParameterGaps(parameters map[string]any) []ParameterGap {
	ids := make([]string, 0, len(parameters))
	for id := range parameters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var gaps []ParameterGap
	for _, id := range ids {
		p, _ := parameters[id].(map[string]any)
		if p == nil {
			continue
		}
		if _, ok := p["default"]; ok {
			continue
		}
		phase, _ := p["phase"].(string)
		typ, _ := p["type"].(string)
		if phase == "" {
			phase = "deploy"
		}
		if typ == "" {
			typ = "string"
		}
		gaps = append(gaps, ParameterGap{
			ID: id, Phase: phase, Type: typ, Reason: "no-default",
		})
	}
	if gaps == nil {
		gaps = []ParameterGap{}
	}
	return gaps
}

func dedupeSuggestions(in []SecretSuggestion) []SecretSuggestion {
	type key struct{ id, source, path, pointer, service, target, envKey, envFile string }
	seen := map[key]struct{}{}
	var out []SecretSuggestion
	for _, s := range in {
		k := key{s.ID, s.Source, s.Path, s.Pointer, s.Service, s.Target, s.EnvKey, s.EnvFile}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Pointer < out[j].Pointer
	})
	if out == nil {
		out = []SecretSuggestion{}
	}
	return out
}
