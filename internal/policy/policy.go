// Package policy loads and evaluates compile policy (pure; no network/FS beyond embed).
package policy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
)

//go:embed default.compile.json
var defaultCompileJSON []byte

// APIVersionV1 is the compile policy document version.
const APIVersionV1 = "iacpack.policy/v1"

// KindCompilePolicy is the only v1 policy kind.
const KindCompilePolicy = "CompilePolicy"

// HardCapEntries / HardCapExpandedBytes cannot be raised by policy (TZ §13.2).
const (
	HardCapEntries       = 100000
	HardCapExpandedBytes = 107374182400 // 100 GiB
)

// CompilePolicy is the effective compile-time policy.
type CompilePolicy struct {
	APIVersion   string
	Kind         string
	Name         string
	Registries   []string
	Capabilities Capabilities
	Quotas       Quotas
	Ceilings     ResourceCeilings
}

// Capabilities are v1-supported high-risk toggles only.
type Capabilities struct {
	DevelopmentBuilds bool
	BuildNetwork      bool
	ExternalNetworks  bool
	ExternalVolumes   bool
	IncludeRuntime    bool
	EmbedImages       bool
}

// Quotas are package/archive ceilings (policy may only lower defaults).
type Quotas struct {
	MaxEntries          int
	MaxEntryBytes       int64
	MaxExpandedBytes    int64
	MaxCompressionRatio int
	MaxPathBytes        int
}

// ResourceCeilings limit service resource declarations.
type ResourceCeilings struct {
	CPUs        string
	MemoryBytes int64
}

// Default returns the immutable embedded default compile policy.
func Default() (CompilePolicy, error) {
	return Parse(defaultCompileJSON)
}

// MustDefault panics only on corrupt embed (programmer error).
func MustDefault() CompilePolicy {
	p, err := Default()
	if err != nil {
		panic(err)
	}
	return p
}

// CanonicalBytes returns RFC 8785 JSON for the effective compile policy.
func CanonicalBytes(p CompilePolicy) ([]byte, error) {
	regs := append([]string(nil), p.Registries...)
	sort.Strings(regs)
	m := map[string]any{
		"apiVersion": p.APIVersion,
		"kind":       p.Kind,
		"metadata":   map[string]any{"name": p.Name},
		"spec": map[string]any{
			"registries": map[string]any{"allow": regs},
			"capabilities": map[string]any{
				"developmentBuilds": p.Capabilities.DevelopmentBuilds,
				"buildNetwork":      p.Capabilities.BuildNetwork,
				"externalNetworks":  p.Capabilities.ExternalNetworks,
				"externalVolumes":   p.Capabilities.ExternalVolumes,
				"includeRuntime":    p.Capabilities.IncludeRuntime,
				"embedImages":       p.Capabilities.EmbedImages,
			},
			"quotas": map[string]any{
				"maxEntries":          p.Quotas.MaxEntries,
				"maxEntryBytes":       p.Quotas.MaxEntryBytes,
				"maxExpandedBytes":    p.Quotas.MaxExpandedBytes,
				"maxCompressionRatio": p.Quotas.MaxCompressionRatio,
				"maxPathBytes":        p.Quotas.MaxPathBytes,
			},
			"resourceCeilings": map[string]any{
				"cpus":        p.Ceilings.CPUs,
				"memoryBytes": p.Ceilings.MemoryBytes,
			},
		},
	}
	return canonicaljson.Marshal(m)
}

// Parse decodes a compile policy document from JSON bytes.
func Parse(data []byte) (CompilePolicy, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return CompilePolicy{}, fmt.Errorf("policy json: %w", err)
	}
	return decodeMap(raw)
}

func decodeMap(raw map[string]any) (CompilePolicy, error) {
	api, _ := raw["apiVersion"].(string)
	kind, _ := raw["kind"].(string)
	if api != APIVersionV1 {
		return CompilePolicy{}, fmt.Errorf("unsupported policy apiVersion %q", api)
	}
	if kind != KindCompilePolicy {
		return CompilePolicy{}, fmt.Errorf("unsupported policy kind %q", kind)
	}
	meta, _ := raw["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	spec, ok := raw["spec"].(map[string]any)
	if !ok {
		return CompilePolicy{}, fmt.Errorf("policy missing spec")
	}
	regsObj, _ := spec["registries"].(map[string]any)
	allowAny, _ := regsObj["allow"].([]any)
	regs := make([]string, 0, len(allowAny))
	for _, a := range allowAny {
		s, ok := a.(string)
		if !ok || s == "" {
			return CompilePolicy{}, fmt.Errorf("invalid registry allow entry")
		}
		regs = append(regs, s)
	}
	capsObj, _ := spec["capabilities"].(map[string]any)
	caps := Capabilities{
		DevelopmentBuilds: asBool(capsObj["developmentBuilds"]),
		BuildNetwork:      asBool(capsObj["buildNetwork"]),
		ExternalNetworks:  asBool(capsObj["externalNetworks"]),
		ExternalVolumes:   asBool(capsObj["externalVolumes"]),
		IncludeRuntime:    asBool(capsObj["includeRuntime"]),
		EmbedImages:       asBool(capsObj["embedImages"]),
	}
	qObj, _ := spec["quotas"].(map[string]any)
	quotas := Quotas{
		MaxEntries:          asInt(qObj["maxEntries"]),
		MaxEntryBytes:       asInt64(qObj["maxEntryBytes"]),
		MaxExpandedBytes:    asInt64(qObj["maxExpandedBytes"]),
		MaxCompressionRatio: asInt(qObj["maxCompressionRatio"]),
		MaxPathBytes:        asInt(qObj["maxPathBytes"]),
	}
	if quotas.MaxEntries > HardCapEntries || quotas.MaxExpandedBytes > HardCapExpandedBytes {
		return CompilePolicy{}, fmt.Errorf("policy quotas exceed hard caps")
	}
	ceil := ResourceCeilings{}
	if cObj, ok := spec["resourceCeilings"].(map[string]any); ok {
		ceil.CPUs, _ = cObj["cpus"].(string)
		ceil.MemoryBytes = asInt64(cObj["memoryBytes"])
	}
	return CompilePolicy{
		APIVersion:   api,
		Kind:         kind,
		Name:         name,
		Registries:   regs,
		Capabilities: caps,
		Quotas:       quotas,
		Ceilings:     ceil,
	}, nil
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case int:
		return n
	default:
		return 0
	}
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// Evaluate checks a validated Project against compile policy.
func Evaluate(p CompilePolicy, project domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	for id, img := range project.Spec.Images {
		if img.IsBuild() {
			if project.Spec.Mode != domain.ModeDevelopment {
				diags = append(diags, diagnostic.Error(diagnostic.CodePolicyCapabilityDenied,
					fmt.Sprintf("image %q uses build but mode is %q", id, project.Spec.Mode)).
					WithHint("Build images are development-only"))
			}
			if !p.Capabilities.DevelopmentBuilds {
				diags = append(diags, diagnostic.Error(diagnostic.CodePolicyCapabilityDenied,
					fmt.Sprintf("image %q build forbidden by compile policy", id)))
			}
			// Build network is not a per-image field in v1; capability is recorded for later phases.
		} else if img.Ref != "" {
			reg := registryOf(img.Ref)
			if reg != "" && !registryAllowed(p.Registries, reg) {
				diags = append(diags, diagnostic.Error(diagnostic.CodePolicyRegistryDenied,
					fmt.Sprintf("image %q registry %q is not allowlisted", id, reg)).
					WithHint("Allowed registries are configured in compile policy"))
			}
		}
	}
	for id, n := range project.Spec.Networks {
		if n.IsExternal() && !p.Capabilities.ExternalNetworks {
			diags = append(diags, diagnostic.Error(diagnostic.CodePolicyCapabilityDenied,
				fmt.Sprintf("network %q is external but compile policy forbids external networks", id)))
		}
	}
	for id, v := range project.Spec.Volumes {
		if v.IsExternal() && !p.Capabilities.ExternalVolumes {
			diags = append(diags, diagnostic.Error(diagnostic.CodePolicyCapabilityDenied,
				fmt.Sprintf("volume %q is external but compile policy forbids external volumes", id)))
		}
	}
	for _, id := range sortedServiceIDs(project.Spec.Services) {
		svc := project.Spec.Services[id]
		if svc.Resources == nil {
			continue
		}
		diags = append(diags, checkResourceCeilings(p.Ceilings, id, *svc.Resources)...)
	}
	if project.Spec.Package.Set {
		if project.Spec.Package.IncludeRuntime && !p.Capabilities.IncludeRuntime {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPackageOptionConflict,
				"package.includeRuntime is true but compile policy does not allow includeRuntime").
				WithHint("Use a compile policy that enables includeRuntime for distribution builds, or set includeRuntime: false"))
		}
		if project.Spec.Package.EmbedImages && !p.Capabilities.EmbedImages {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPackageOptionConflict,
				"package.embedImages is true but compile policy does not allow embedImages"))
		}
	}
	diagnostic.SortDeterministic(diags)
	return diags
}

func checkResourceCeilings(ceil ResourceCeilings, serviceID string, rl domain.ResourceLimits) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	if ceil.CPUs != "" && rl.CPUs != "" {
		want, wantOK := parseCPUDecimal(ceil.CPUs)
		got, gotOK := parseCPUDecimal(rl.CPUs)
		if wantOK && gotOK && got > want {
			diags = append(diags, diagnostic.Error(diagnostic.CodePolicyResourceCeiling,
				fmt.Sprintf("service %q resources.cpus %s exceeds compile policy ceiling %s", serviceID, rl.CPUs, ceil.CPUs)).
				WithHint("Lower service.resources.cpus or raise resourceCeilings in compile policy (within schema limits)"))
		}
	}
	if ceil.MemoryBytes > 0 && rl.Memory > 0 && rl.Memory > ceil.MemoryBytes {
		diags = append(diags, diagnostic.Error(diagnostic.CodePolicyResourceCeiling,
			fmt.Sprintf("service %q resources.memory %d exceeds compile policy ceiling %d", serviceID, rl.Memory, ceil.MemoryBytes)).
			WithHint("Lower service.resources.memory or raise resourceCeilings.memoryBytes in compile policy"))
	}
	return diags
}

func parseCPUDecimal(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return f, true
}

func sortedServiceIDs(m map[string]domain.Service) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// PackageFormatConflict reports CLI format vs package option conflicts (TZ §9.14).
func PackageFormatConflict(pkg domain.Package, format string) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || !pkg.Set {
		return nil
	}
	switch format {
	case "distribution":
		if !pkg.IncludeRuntime {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPackageOptionConflict,
				"format distribution requires package.includeRuntime: true"))
		}
	case "directory", "bundle", "online":
		if pkg.IncludeRuntime {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPackageOptionConflict,
				fmt.Sprintf("format %s conflicts with package.includeRuntime: true", format)))
		}
		if format == "directory" && pkg.EmbedImages {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPackageOptionConflict,
				"format directory conflicts with package.embedImages: true"))
		}
	}
	diagnostic.SortDeterministic(diags)
	return diags
}

func registryOf(ref string) string {
	// Strip digest/tag for host detection.
	base := ref
	if i := strings.Index(ref, "@"); i >= 0 {
		base = ref[:i]
	} else if i := strings.LastIndex(ref, ":"); i >= 0 {
		// careful: registry:port/repo — only strip tag after last slash
		if slash := strings.LastIndex(ref, "/"); slash >= 0 && i > slash {
			base = ref[:i]
		}
	}
	first, _, ok := strings.Cut(base, "/")
	if !ok {
		return "docker.io"
	}
	if !strings.Contains(first, ".") && first != "localhost" && !strings.Contains(first, ":") {
		return "docker.io"
	}
	// host:port
	host := first
	if h, _, ok := strings.Cut(first, ":"); ok {
		host = h
	}
	return strings.ToLower(host)
}

func registryAllowed(allow []string, reg string) bool {
	reg = strings.ToLower(reg)
	for _, a := range allow {
		if strings.ToLower(a) == reg {
			return true
		}
	}
	return false
}
