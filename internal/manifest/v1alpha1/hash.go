package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	domain "iacpack/internal/domain/manifest"
)

// SemanticHash returns a stable SHA-256 hex digest of deployment semantics,
// excluding spec.x-ui (ADR-012).
func SemanticHash(p domain.Project) (string, error) {
	canon, err := CanonicalSemantic(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// CanonicalSemantic returns deterministic JSON bytes of public deployment semantics.
func CanonicalSemantic(p domain.Project) ([]byte, error) {
	m := map[string]any{
		"apiVersion": p.APIVersion,
		"kind":       p.Kind,
		"metadata": map[string]any{
			"name":        p.Metadata.Name,
			"version":     p.Metadata.Version,
			"description": p.Metadata.Description,
		},
		"spec": semanticSpec(p.Spec),
	}
	return marshalSorted(m)
}

func semanticSpec(s domain.Spec) map[string]any {
	out := map[string]any{
		"target": map[string]any{"profile": string(s.Target.Profile)},
		"mode":   string(s.Mode),
	}
	if len(s.Parameters) > 0 {
		out["parameters"] = mapOf(s.Parameters, func(p domain.Parameter) any {
			m := map[string]any{"phase": string(p.Phase), "type": string(p.Type)}
			if p.Default != nil {
				m["default"] = scalarJSON(*p.Default)
			}
			if p.Minimum != nil {
				m["minimum"] = *p.Minimum
			}
			if p.Maximum != nil {
				m["maximum"] = *p.Maximum
			}
			if p.Pattern != "" {
				m["pattern"] = p.Pattern
			}
			if len(p.Choices) > 0 {
				ch := make([]any, len(p.Choices))
				for i, c := range p.Choices {
					ch[i] = scalarJSON(c)
				}
				m["choices"] = ch
			}
			if p.Description != "" {
				m["description"] = p.Description
			}
			return m
		})
	}
	if len(s.Secrets) > 0 {
		out["secrets"] = mapOf(s.Secrets, func(sec domain.Secret) any {
			m := map[string]any{
				"required": sec.Required,
				"delivery": sec.Delivery,
			}
			if sec.Description != "" {
				m["description"] = sec.Description
			}
			return m
		})
	}
	out["images"] = mapOf(s.Images, func(img domain.Image) any {
		if img.Build != nil {
			b := map[string]any{
				"context":    img.Build.Context.String(),
				"dockerfile": img.Build.Dockerfile.String(),
				"include":    img.Build.Include,
			}
			if len(img.Build.Exclude) > 0 {
				b["exclude"] = img.Build.Exclude
			}
			if len(img.Build.Args) > 0 {
				b["args"] = mapOf(img.Build.Args, valueSourceJSON)
			}
			if img.Build.ExpectedImageID != "" {
				b["expectedImageID"] = img.Build.ExpectedImageID
			}
			return map[string]any{"build": b}
		}
		ref := any(img.Ref)
		if img.RefParameter != "" {
			ref = map[string]any{"parameter": img.RefParameter}
		}
		return map[string]any{"ref": ref, "delivery": img.Delivery}
	})
	if len(s.Builds) > 0 {
		out["builds"] = mapOf(s.Builds, func(b domain.PackageBuild) any {
			return map[string]any{
				"context":        b.Context.String(),
				"packageManager": b.PackageManager,
				"install":        b.Install,
				"script":         b.Script,
				"output":         b.Output,
				"entry":          b.Entry,
			}
		})
	}
	if len(s.Resources) > 0 {
		out["resources"] = mapOf(s.Resources, func(r domain.Resource) any {
			m := map[string]any{
				"source":     r.Source.String(),
				"include":    r.Include,
				"allowEmpty": r.AllowEmpty,
			}
			if len(r.Exclude) > 0 {
				m["exclude"] = r.Exclude
			}
			if r.Destination != "" {
				m["destination"] = r.Destination.String()
			}
			if r.Mode != "" {
				m["mode"] = r.Mode
			}
			return m
		})
	}
	if len(s.Networks) > 0 {
		out["networks"] = mapOf(s.Networks, func(n domain.Network) any {
			if n.External != nil {
				return map[string]any{"external": map[string]any{"name": n.External.Name}}
			}
			return map[string]any{}
		})
	}
	if len(s.Volumes) > 0 {
		out["volumes"] = mapOf(s.Volumes, func(v domain.Volume) any {
			if v.External != nil {
				return map[string]any{"external": map[string]any{"name": v.External.Name}}
			}
			ret := v.Retention
			if ret == "" {
				ret = "retain"
			}
			return map[string]any{"retention": ret}
		})
	}
	if len(s.Databases) > 0 {
		out["databases"] = mapOf(s.Databases, func(db domain.Database) any {
			return map[string]any{
				"on": db.On, "name": db.Name, "ownerFrom": db.OwnerFrom,
				"migrations": db.Migrations, "urlParameter": db.URLParameter,
			}
		})
	}
	out["services"] = mapOf(s.Services, serviceJSON)
	if len(s.Tasks) > 0 {
		out["tasks"] = mapOf(s.Tasks, taskJSON)
	}
	if s.Package.Set {
		out["package"] = map[string]any{
			"generateReadme":     s.Package.GenerateReadme,
			"generateEnvExample": s.Package.GenerateEnvExample,
			"includeRuntime":     s.Package.IncludeRuntime,
			"embedImages":        s.Package.EmbedImages,
		}
	}
	// intentionally omit x-ui
	return out
}

func serviceJSON(svc domain.Service) any {
	m := map[string]any{"image": svc.Image, "restart": svc.Restart}
	if svc.User != "" {
		m["user"] = svc.User
	}
	if len(svc.Command) > 0 {
		m["command"] = argvJSON(svc.Command)
	}
	if len(svc.Entrypoint) > 0 {
		m["entrypoint"] = argvJSON(svc.Entrypoint)
	}
	if len(svc.Environment) > 0 {
		m["environment"] = mapOf(svc.Environment, valueSourceJSON)
	}
	if svc.EnvironmentFrom != "" {
		m["environmentFrom"] = svc.EnvironmentFrom
	}
	if len(svc.Secrets) > 0 {
		arr := make([]any, len(svc.Secrets))
		for i, sm := range svc.Secrets {
			arr[i] = map[string]any{"ref": sm.Ref, "target": sm.Target.String()}
		}
		m["secrets"] = arr
	}
	if len(svc.Ports) > 0 {
		arr := make([]any, len(svc.Ports))
		for i, p := range svc.Ports {
			pm := map[string]any{"container": p.Container, "protocol": p.Protocol}
			if p.Bind != "" {
				pm["bind"] = p.Bind
			}
			if p.Host != nil {
				if p.Host.Literal != nil {
					pm["host"] = *p.Host.Literal
				} else if p.Host.Parameter != "" {
					pm["host"] = map[string]any{"parameter": p.Host.Parameter}
				}
			}
			arr[i] = pm
		}
		m["ports"] = arr
	}
	if len(svc.Mounts) > 0 {
		m["mounts"] = mountsJSON(svc.Mounts)
	}
	if len(svc.Networks) > 0 {
		m["networks"] = append([]string(nil), svc.Networks...)
	}
	if svc.Healthcheck != nil {
		m["healthcheck"] = healthJSON(*svc.Healthcheck)
	}
	if svc.Resources != nil {
		rm := map[string]any{}
		if svc.Resources.CPUs != "" {
			rm["cpus"] = svc.Resources.CPUs
		}
		if svc.Resources.Memory > 0 {
			rm["memory"] = svc.Resources.Memory
		}
		if svc.Resources.GPUAll {
			rm["gpu"] = "all"
		} else if svc.Resources.GPUDevicesOnly {
			rm["gpu"] = "devices-only"
			if svc.Resources.LDLibraryPath != "" {
				rm["ldLibraryPath"] = svc.Resources.LDLibraryPath
			}
		} else if svc.Resources.GPU > 0 {
			rm["gpu"] = svc.Resources.GPU
		}
		m["resources"] = rm
	}
	if len(svc.DependsOn) > 0 {
		m["dependsOn"] = depsJSON(svc.DependsOn)
	}
	if len(svc.Profiles) > 0 {
		m["profiles"] = append([]string(nil), svc.Profiles...)
	}
	return m
}

func taskJSON(t domain.Task) any {
	m := map[string]any{
		"phase":   string(t.Phase),
		"image":   t.Image,
		"command": argvJSON(t.Command),
		"rerun":   t.Rerun,
	}
	if t.User != "" {
		m["user"] = t.User
	}
	if t.ResetEntrypoint {
		m["entrypoint"] = []any{}
	} else if len(t.Entrypoint) > 0 {
		m["entrypoint"] = argvJSON(t.Entrypoint)
	}
	if t.Workdir != "" {
		m["workdir"] = t.Workdir.String()
	}
	if t.Timeout != "" {
		m["timeout"] = t.Timeout
	}
	if t.Retry != nil {
		rm := map[string]any{"count": t.Retry.Count}
		if t.Retry.InitialBackoff != "" {
			rm["initialBackoff"] = t.Retry.InitialBackoff
		}
		if t.Retry.Multiplier != 0 {
			rm["multiplier"] = t.Retry.Multiplier
		}
		if t.Retry.MaxBackoff != "" {
			rm["maxBackoff"] = t.Retry.MaxBackoff
		}
		m["retry"] = rm
	}
	if len(t.DependsOn) > 0 {
		m["dependsOn"] = depsJSON(t.DependsOn)
	}
	if len(t.Mounts) > 0 {
		m["mounts"] = mountsJSON(t.Mounts)
	}
	if len(t.Networks) > 0 {
		m["networks"] = append([]string(nil), t.Networks...)
	}
	if len(t.Secrets) > 0 {
		arr := make([]any, len(t.Secrets))
		for i, sm := range t.Secrets {
			arr[i] = map[string]any{"ref": sm.Ref, "target": sm.Target.String()}
		}
		m["secrets"] = arr
	}
	return m
}

func mountsJSON(ms []domain.Mount) []any {
	arr := make([]any, len(ms))
	for i, m := range ms {
		mm := map[string]any{"target": m.Target.String(), "readOnly": m.ReadOnly}
		if m.Volume != "" {
			mm["volume"] = m.Volume
		}
		if m.Resource != "" {
			mm["resource"] = m.Resource
			if m.Source != "" {
				mm["source"] = m.Source
			}
		}
		if m.Build != "" {
			mm["build"] = m.Build
		}
		if m.HostLiteral != "" {
			mm["host"] = m.HostLiteral
		} else if m.HostParameter != "" {
			mm["host"] = map[string]any{"parameter": m.HostParameter}
		}
		arr[i] = mm
	}
	return arr
}

func depsJSON(ds []domain.Dependency) []any {
	arr := make([]any, len(ds))
	for i, d := range ds {
		arr[i] = map[string]any{"target": d.Target, "condition": d.Condition}
	}
	return arr
}

func healthJSON(h domain.Healthcheck) map[string]any {
	m := map[string]any{
		"command":  h.Command,
		"interval": h.Interval,
		"timeout":  h.Timeout,
		"retries":  h.Retries,
	}
	if h.StartPeriod != "" {
		m["startPeriod"] = h.StartPeriod
	}
	return m
}

func argvJSON(cmd []domain.ArgvElement) []any {
	out := make([]any, len(cmd))
	for i, el := range cmd {
		if len(el.Parts) == 0 {
			if el.Secret != "" {
				out[i] = map[string]any{"secret": el.Secret}
				continue
			}
			if el.Literal == "" {
				out[i] = map[string]any{"literal": ""}
				continue
			}
			out[i] = el.Literal
			continue
		}
		parts := make([]any, len(el.Parts))
		for j, p := range el.Parts {
			switch {
			case p.Secret != "":
				parts[j] = map[string]any{"secret": p.Secret}
			case p.Parameter != "":
				parts[j] = map[string]any{"parameter": p.Parameter}
			default:
				parts[j] = p.Literal
			}
		}
		out[i] = map[string]any{"parts": parts}
	}
	return out
}

func valueSourceJSON(vs domain.ValueSource) any {
	if len(vs.Parts) > 0 {
		parts := make([]any, len(vs.Parts))
		for i, p := range vs.Parts {
			parts[i] = valueSourceJSON(p)
		}
		return map[string]any{"parts": parts}
	}
	if vs.Secret != "" {
		return map[string]any{"secret": vs.Secret}
	}
	if vs.Parameter != "" {
		return map[string]any{"parameter": vs.Parameter}
	}
	if vs.Literal != nil {
		return map[string]any{"literal": scalarJSON(*vs.Literal)}
	}
	return map[string]any{}
}

func scalarJSON(sc domain.Scalar) any {
	switch sc.Kind {
	case domain.ScalarString:
		return sc.Str
	case domain.ScalarNumber:
		return json.Number(sc.Str)
	case domain.ScalarBool:
		return sc.Bool
	default:
		return nil
	}
}

func mapOf[T any](m map[string]T, fn func(T) any) map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = fn(m[k])
	}
	return out
}

func marshalSorted(v any) ([]byte, error) {
	// encoding/json map iteration is randomized; rebuild via sorted encoding.
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(b, &normalized); err != nil {
		return nil, err
	}
	return encodeCanonical(normalized)
}

func encodeCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte("{")
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := encodeCanonical(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte("[")
		for i, e := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			eb, err := encodeCanonical(e)
			if err != nil {
				return nil, err
			}
			buf = append(buf, eb...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(t)
	}
}

// PublicInspectModel returns the public inspect payload (no secret values).
func PublicInspectModel(p domain.Project) (map[string]any, error) {
	raw, err := CanonicalSemantic(p)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	hash, err := SemanticHash(p)
	if err != nil {
		return nil, err
	}
	m["semanticHash"] = hash
	if p.Spec.XUI != nil {
		xui := map[string]any{}
		if len(p.Spec.XUI.Nodes) > 0 {
			xui["nodes"] = mapOf(p.Spec.XUI.Nodes, func(n domain.XUINode) any {
				nm := map[string]any{"x": n.X, "y": n.Y}
				if n.Category != "" {
					nm["category"] = n.Category
				}
				return nm
			})
		}
		if p.Spec.XUI.Viewport != nil {
			xui["viewport"] = map[string]any{
				"x": p.Spec.XUI.Viewport.X, "y": p.Spec.XUI.Viewport.Y, "zoom": p.Spec.XUI.Viewport.Zoom,
			}
		}
		if spec, ok := m["spec"].(map[string]any); ok {
			spec["x-ui"] = xui
		}
	}
	return m, nil
}

// FormatInspectText renders a stable text inspect view.
func FormatInspectText(p domain.Project) (string, error) {
	hash, err := SemanticHash(p)
	if err != nil {
		return "", err
	}
	var b []byte
	b = append(b, fmt.Sprintf("project: %s@%s\n", p.Metadata.Name, p.Metadata.Version)...)
	b = append(b, fmt.Sprintf("mode: %s\n", p.Spec.Mode)...)
	b = append(b, fmt.Sprintf("profile: %s\n", p.Spec.Target.Profile)...)
	b = append(b, fmt.Sprintf("semanticHash: %s\n", hash)...)
	b = append(b, fmt.Sprintf("services: %d\n", len(p.Spec.Services))...)
	b = append(b, fmt.Sprintf("tasks: %d\n", len(p.Spec.Tasks))...)
	b = append(b, fmt.Sprintf("images: %d\n", len(p.Spec.Images))...)
	b = append(b, fmt.Sprintf("secrets: %d (declarations only)\n", len(p.Spec.Secrets))...)
	return string(b), nil
}
