// Package oppolicy evaluates local operator policy that may only tighten compile policy.
package oppolicy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/policy"
)

// KindOperatorPolicy is the local plan/apply policy kind.
const KindOperatorPolicy = "OperatorPolicy"

// Policy is a local operator policy document.
type Policy struct {
	APIVersion    string
	Kind          string
	Name          string
	Registries    []string // if non-empty, must be subset of compile allowlist
	Capabilities  CapTightening
	AllowUnsigned bool // default false
}

// CapTightening can only disable compile-enabled capabilities.
type CapTightening struct {
	DevelopmentBuilds *bool
	BuildNetwork      *bool
	ExternalNetworks  *bool
	ExternalVolumes   *bool
	IncludeRuntime    *bool
	EmbedImages       *bool
}

// Default returns the immutable operator default (strict; unsigned denied).
func Default() Policy {
	return Policy{
		APIVersion:    policy.APIVersionV1,
		Kind:          KindOperatorPolicy,
		Name:          "default-operator",
		AllowUnsigned: false,
	}
}

// Parse decodes an operator policy JSON document.
func Parse(data []byte) (Policy, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Policy{}, fmt.Errorf("operator policy json: %w", err)
	}
	api, _ := raw["apiVersion"].(string)
	kind, _ := raw["kind"].(string)
	if api != policy.APIVersionV1 {
		return Policy{}, fmt.Errorf("unsupported operator policy apiVersion %q", api)
	}
	if kind != KindOperatorPolicy {
		return Policy{}, fmt.Errorf("unsupported operator policy kind %q", kind)
	}
	p := Default()
	if md, ok := raw["metadata"].(map[string]any); ok {
		if n, ok := md["name"].(string); ok {
			p.Name = n
		}
	}
	spec, _ := raw["spec"].(map[string]any)
	if spec == nil {
		return p, nil
	}
	if regs, ok := spec["registries"].(map[string]any); ok {
		if allow, ok := regs["allow"].([]any); ok {
			for _, a := range allow {
				s, ok := a.(string)
				if !ok || s == "" {
					return Policy{}, fmt.Errorf("invalid registry allow entry")
				}
				p.Registries = append(p.Registries, s)
			}
			sort.Strings(p.Registries)
		}
	}
	if caps, ok := spec["capabilities"].(map[string]any); ok {
		for k := range caps {
			switch k {
			case "developmentBuilds", "buildNetwork", "externalNetworks", "externalVolumes", "includeRuntime", "embedImages":
			default:
				return Policy{}, fmt.Errorf("unsupported operator capability %q", k)
			}
		}
		p.Capabilities.DevelopmentBuilds = boolPtr(caps, "developmentBuilds")
		p.Capabilities.BuildNetwork = boolPtr(caps, "buildNetwork")
		p.Capabilities.ExternalNetworks = boolPtr(caps, "externalNetworks")
		p.Capabilities.ExternalVolumes = boolPtr(caps, "externalVolumes")
		p.Capabilities.IncludeRuntime = boolPtr(caps, "includeRuntime")
		p.Capabilities.EmbedImages = boolPtr(caps, "embedImages")
	}
	if v, ok := spec["allowUnsigned"].(bool); ok {
		p.AllowUnsigned = v
	}
	return p, nil
}

func boolPtr(m map[string]any, key string) *bool {
	v, ok := m[key]
	if !ok {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

const (
	CodeOpPolicyWeaken = "POLICY.OPERATOR_WEAKEN"
	CodeOpPolicyDeny   = "POLICY.OPERATOR_DENY"
)

// Evaluate ensures operator policy is stricter-or-equal vs compile and allows the deployment.
func Evaluate(op Policy, compile policy.CompilePolicy, dep ir.Deployment) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	if len(op.Registries) > 0 {
		allow := map[string]bool{}
		for _, r := range compile.Registries {
			allow[r] = true
		}
		for _, r := range op.Registries {
			if !allow[r] {
				diags = append(diags, diagnostic.Error(CodeOpPolicyWeaken,
					fmt.Sprintf("operator policy registry %q is not in compile allowlist", r)))
			}
		}
		// Images must be under operator allowlist when set.
		opAllow := map[string]bool{}
		for _, r := range op.Registries {
			opAllow[r] = true
		}
		for id, img := range dep.Images {
			if img.Ref == "" {
				continue
			}
			reg := registryOf(img.Ref)
			if !opAllow[reg] {
				diags = append(diags, diagnostic.Error(CodeOpPolicyDeny,
					fmt.Sprintf("image %q registry %q denied by operator policy", id, reg)))
			}
		}
	}
	diags = append(diags, tighten("developmentBuilds", op.Capabilities.DevelopmentBuilds, compile.Capabilities.DevelopmentBuilds)...)
	diags = append(diags, tighten("buildNetwork", op.Capabilities.BuildNetwork, compile.Capabilities.BuildNetwork)...)
	diags = append(diags, tighten("externalNetworks", op.Capabilities.ExternalNetworks, compile.Capabilities.ExternalNetworks)...)
	diags = append(diags, tighten("externalVolumes", op.Capabilities.ExternalVolumes, compile.Capabilities.ExternalVolumes)...)
	diags = append(diags, tighten("includeRuntime", op.Capabilities.IncludeRuntime, compile.Capabilities.IncludeRuntime)...)
	diags = append(diags, tighten("embedImages", op.Capabilities.EmbedImages, compile.Capabilities.EmbedImages)...)

	// Effective external denies
	extNet := compile.Capabilities.ExternalNetworks
	if op.Capabilities.ExternalNetworks != nil {
		extNet = *op.Capabilities.ExternalNetworks && extNet
	}
	extVol := compile.Capabilities.ExternalVolumes
	if op.Capabilities.ExternalVolumes != nil {
		extVol = *op.Capabilities.ExternalVolumes && extVol
	}
	if !extNet {
		for id, n := range dep.Networks {
			if n.External != nil {
				diags = append(diags, diagnostic.Error(CodeOpPolicyDeny,
					fmt.Sprintf("external network %q denied by operator/compile policy", id)))
			}
		}
	}
	if !extVol {
		for id, v := range dep.Volumes {
			if v.External != nil {
				diags = append(diags, diagnostic.Error(CodeOpPolicyDeny,
					fmt.Sprintf("external volume %q denied by operator/compile policy", id)))
			}
		}
	}
	diagnostic.SortDeterministic(diags)
	return diags
}

func tighten(name string, op *bool, compileEnabled bool) []diagnostic.Diagnostic {
	if op == nil {
		return nil
	}
	if *op && !compileEnabled {
		return []diagnostic.Diagnostic{diagnostic.Error(CodeOpPolicyWeaken,
			fmt.Sprintf("operator policy cannot enable %s when compile policy disables it", name))}
	}
	return nil
}

func registryOf(ref string) string {
	base := ref
	if i := strings.Index(ref, "@"); i >= 0 {
		base = ref[:i]
	} else if i := strings.LastIndex(ref, ":"); i >= 0 {
		if slash := strings.LastIndex(ref, "/"); slash >= 0 && i > slash {
			base = ref[:i]
		}
	}
	parts := strings.Split(base, "/")
	if len(parts) == 1 {
		return "docker.io"
	}
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		return parts[0]
	}
	return "docker.io"
}
