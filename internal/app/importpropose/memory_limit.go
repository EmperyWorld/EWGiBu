package importpropose

import (
	"fmt"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

// composeMemoryBytesFromNode parses Compose memory limits (deploy.limits.memory or mem_limit).
func composeMemoryBytesFromNode(n *loader.Node) (bytes int64, ok bool, diags []diagnostic.Diagnostic) {
	if n == nil {
		return 0, false, nil
	}
	switch n.Kind {
	case loader.KindString:
		raw := strings.TrimSpace(n.Str)
		if lit, resolved := resolveImportLiteralScalar(raw); resolved {
			raw = lit
		} else if interpRe.MatchString(raw) {
			return 0, false, []diagnostic.Diagnostic{
				diagnostic.Warning(diagnostic.CodeImportInterpolation,
					"memory limit with unresolved Compose interpolation omitted").
					WithSource("", "", n.Line, n.Column),
			}
		}
		b, err := parseComposeMemoryBytes(raw)
		if err != nil {
			return 0, false, []diagnostic.Diagnostic{
				diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("memory limit %q omitted; %v", raw, err)).
					WithSource("", "", n.Line, n.Column),
			}
		}
		return b, true, nil
	case loader.KindNumber:
		if v, err := parseComposeMemoryBytes(n.Str); err == nil {
			return v, true, nil
		}
		return 0, false, nil
	default:
		return 0, false, nil
	}
}

func applyServiceMemoryLimit(
	svc map[string]any,
	sid, file, pointer string,
	n *loader.Node,
	diags *[]diagnostic.Diagnostic,
) {
	b, ok, md := composeMemoryBytesFromNode(n)
	*diags = append(*diags, md...)
	if !ok {
		return
	}
	res := serviceResourcesMap(svc)
	if existing, _ := res["memory"].(int64); existing > 0 && existing != b {
		*diags = append(*diags, diagnostic.Error(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: mem_limit conflicts with deploy.resources.limits.memory", sid)).
			WithSource(file, pointer, n.Line, n.Column))
		return
	}
	if existingInt, _ := res["memory"].(int); existingInt > 0 && int64(existingInt) != b {
		*diags = append(*diags, diagnostic.Error(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: mem_limit conflicts with deploy.resources.limits.memory", sid)).
			WithSource(file, pointer, n.Line, n.Column))
		return
	}
	res["memory"] = b
}

func serviceResourcesMap(svc map[string]any) map[string]any {
	raw, _ := svc["resources"].(map[string]any)
	if raw == nil {
		raw = map[string]any{}
		svc["resources"] = raw
	}
	return raw
}
