package compile

import (
	"sort"

	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/manifest/v1alpha1"
)

// ToIR converts a validated authoring Project into immutable IR.
// Call only after schema/semantic/parameter/input/policy validation.
func ToIR(p domain.Project) (ir.Deployment, error) {
	hash, err := v1alpha1.SemanticHash(p)
	if err != nil {
		return ir.Deployment{}, err
	}
	d := ir.Deployment{
		FormatVersion: ir.FormatVersion,
		Project: ir.ProjectRef{
			Name:        p.Metadata.Name,
			Version:     p.Metadata.Version,
			Description: p.Metadata.Description,
		},
		Mode:         string(p.Spec.Mode),
		Profile:      string(p.Spec.Target.Profile),
		SemanticHash: hash,
		Parameters:   map[string]ir.Parameter{},
		Secrets:      map[string]ir.Secret{},
		Images:       map[string]ir.Image{},
		Resources:    map[string]ir.Resource{},
		Networks:     map[string]ir.Network{},
		Volumes:      map[string]ir.Volume{},
		Services:     map[string]ir.Service{},
		Tasks:        map[string]ir.Task{},
	}
	for _, id := range sortedKeys(p.Spec.Parameters) {
		pr := p.Spec.Parameters[id]
		ip := ir.Parameter{Phase: string(pr.Phase), Type: string(pr.Type), Description: pr.Description}
		if pr.Default != nil {
			ip.Default = scalarAny(*pr.Default)
		}
		d.Parameters[id] = ip
	}
	for _, id := range sortedKeys(p.Spec.Secrets) {
		s := p.Spec.Secrets[id]
		d.Secrets[id] = ir.Secret{Required: s.Required, Delivery: s.Delivery, Description: s.Description}
	}
	for _, id := range sortedKeys(p.Spec.Images) {
		img := p.Spec.Images[id]
		out := ir.Image{Ref: img.Ref, RefParameter: img.RefParameter, Delivery: img.Delivery}
		if img.Build != nil {
			out.Build = &ir.ImageBuild{
				Context:    img.Build.Context.String(),
				Dockerfile: img.Build.Dockerfile.String(),
				Include:    append([]string(nil), img.Build.Include...),
				Exclude:    append([]string(nil), img.Build.Exclude...),
			}
		}
		d.Images[id] = out
	}
	if len(p.Spec.Builds) > 0 {
		d.Builds = map[string]ir.PackageBuild{}
		for _, id := range sortedKeys(p.Spec.Builds) {
			b := p.Spec.Builds[id]
			d.Builds[id] = ir.PackageBuild{
				Context: b.Context.String(), PackageManager: b.PackageManager, Install: b.Install,
				Script: b.Script, Output: b.Output, Entry: b.Entry,
			}
		}
	}
	for _, id := range sortedKeys(p.Spec.Resources) {
		r := p.Spec.Resources[id]
		d.Resources[id] = ir.Resource{
			Source:      r.Source.String(),
			Include:     append([]string(nil), r.Include...),
			Exclude:     append([]string(nil), r.Exclude...),
			Destination: r.Destination.String(),
			Mode:        r.Mode,
			AllowEmpty:  r.AllowEmpty,
		}
	}
	for _, id := range sortedKeys(p.Spec.Networks) {
		n := p.Spec.Networks[id]
		out := ir.Network{}
		if n.External != nil {
			out.External = &ir.ExternalRef{Name: n.External.Name}
		}
		d.Networks[id] = out
	}
	for _, id := range sortedKeys(p.Spec.Volumes) {
		v := p.Spec.Volumes[id]
		out := ir.Volume{Retention: v.Retention}
		if out.Retention == "" && v.External == nil {
			out.Retention = "retain"
		}
		if v.External != nil {
			out.External = &ir.ExternalRef{Name: v.External.Name}
		}
		d.Volumes[id] = out
	}
	for _, id := range sortedKeys(p.Spec.Services) {
		d.Services[id] = convertService(p.Spec.Services[id])
	}
	for _, id := range sortedKeys(p.Spec.Tasks) {
		d.Tasks[id] = convertTask(p.Spec.Tasks[id])
	}
	if p.Spec.Package.Set {
		d.Package = ir.Package{
			GenerateReadme:     p.Spec.Package.GenerateReadme,
			GenerateEnvExample: p.Spec.Package.GenerateEnvExample,
			IncludeRuntime:     p.Spec.Package.IncludeRuntime,
			EmbedImages:        p.Spec.Package.EmbedImages,
		}
	} else {
		d.Package = ir.Package{GenerateReadme: true, GenerateEnvExample: true}
	}
	if err := expandDatabases(&d, p); err != nil {
		return ir.Deployment{}, err
	}
	return d, nil
}

func convertService(s domain.Service) ir.Service {
	out := ir.Service{
		Image:           s.Image,
		User:            s.User,
		Command:         convertArgv(s.Command),
		Entrypoint:      convertArgv(s.Entrypoint),
		EnvironmentFrom: s.EnvironmentFrom,
		Restart:         s.Restart,
		Networks:        append([]string(nil), s.Networks...),
	}
	if len(s.Environment) > 0 {
		out.Environment = map[string]ir.ValueSource{}
		for _, k := range sortedKeys(s.Environment) {
			out.Environment[k] = convertVS(s.Environment[k])
		}
	}
	for _, sm := range s.Secrets {
		out.Secrets = append(out.Secrets, ir.SecretMount{Ref: sm.Ref, Target: sm.Target.String()})
	}
	for _, p := range s.Ports {
		ip := ir.Port{Container: p.Container, Bind: p.Bind, Protocol: p.Protocol}
		if p.Host != nil {
			ip.Host = &ir.HostPort{Literal: p.Host.Literal, Parameter: p.Host.Parameter}
		}
		out.Ports = append(out.Ports, ip)
	}
	out.Mounts = convertMounts(s.Mounts)
	if s.Healthcheck != nil {
		h := *s.Healthcheck
		out.Healthcheck = &ir.Healthcheck{
			Command: append([]string(nil), h.Command...), Interval: h.Interval,
			Timeout: h.Timeout, StartPeriod: h.StartPeriod, Retries: h.Retries,
		}
	}
	if s.Resources != nil {
		out.Resources = &ir.ResourceLimits{
			CPUs: s.Resources.CPUs, Memory: s.Resources.Memory, GPU: s.Resources.GPU, GPUAll: s.Resources.GPUAll,
			GPUDevicesOnly: s.Resources.GPUDevicesOnly, LDLibraryPath: s.Resources.LDLibraryPath,
		}
	}
	for _, d := range s.DependsOn {
		out.DependsOn = append(out.DependsOn, ir.Dependency{Target: d.Target, Condition: d.Condition})
	}
	if len(s.Profiles) > 0 {
		out.Profiles = append([]string(nil), s.Profiles...)
	}
	return out
}

func convertTask(t domain.Task) ir.Task {
	out := ir.Task{
		Phase:           string(t.Phase),
		Image:           t.Image,
		Command:         convertArgv(t.Command),
		Entrypoint:      convertArgv(t.Entrypoint),
		ResetEntrypoint: t.ResetEntrypoint,
		User:            t.User,
		Workdir:         t.Workdir.String(),
		Timeout:         t.Timeout,
		Rerun:           t.Rerun,
		Networks:        append([]string(nil), t.Networks...),
	}
	if t.Retry != nil {
		out.Retry = &ir.Retry{
			Count: t.Retry.Count, InitialBackoff: t.Retry.InitialBackoff,
			Multiplier: t.Retry.Multiplier, MaxBackoff: t.Retry.MaxBackoff,
		}
	}
	out.Mounts = convertMounts(t.Mounts)
	for _, sm := range t.Secrets {
		out.Secrets = append(out.Secrets, ir.SecretMount{Ref: sm.Ref, Target: sm.Target.String()})
	}
	for _, d := range t.DependsOn {
		out.DependsOn = append(out.DependsOn, ir.Dependency{Target: d.Target, Condition: d.Condition})
	}
	return out
}

func convertMounts(ms []domain.Mount) []ir.Mount {
	out := make([]ir.Mount, 0, len(ms))
	for _, m := range ms {
		out = append(out, ir.Mount{
			Volume: m.Volume, Resource: m.Resource, Build: m.Build, Source: m.Source,
			HostLiteral: m.HostLiteral, HostParameter: m.HostParameter,
			Target: m.Target.String(), ReadOnly: m.ReadOnly,
		})
	}
	return out
}

func convertArgv(in []domain.ArgvElement) []ir.ArgvElement {
	if len(in) == 0 {
		return nil
	}
	out := make([]ir.ArgvElement, len(in))
	for i, el := range in {
		out[i].Literal = el.Literal
		out[i].Secret = el.Secret
		if len(el.Parts) == 0 {
			continue
		}
		out[i].Parts = make([]ir.ArgvPart, len(el.Parts))
		for j, p := range el.Parts {
			out[i].Parts[j] = ir.ArgvPart{Literal: p.Literal, Parameter: p.Parameter, Secret: p.Secret}
		}
	}
	return out
}

func convertVS(v domain.ValueSource) ir.ValueSource {
	out := ir.ValueSource{Parameter: v.Parameter, Secret: v.Secret}
	if v.Literal != nil {
		out.Literal = scalarAny(*v.Literal)
	}
	if len(v.Parts) > 0 {
		out.Parts = make([]ir.ValueSource, len(v.Parts))
		for i, p := range v.Parts {
			out.Parts[i] = convertVS(p)
		}
	}
	return out
}

func scalarAny(s domain.Scalar) any {
	switch s.Kind {
	case domain.ScalarString:
		return s.Str
	case domain.ScalarNumber:
		return s.Num
	case domain.ScalarBool:
		return s.Bool
	default:
		return nil
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
