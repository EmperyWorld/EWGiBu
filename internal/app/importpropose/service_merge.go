package importpropose

import "iacpack/internal/manifest/loader"

// serviceFragment is a Compose file's contribution to one service id.
// fields lists manifest keys that !override / !reset must replace.
type serviceFragment struct {
	body   map[string]any
	fields map[string]struct{}
}

func composeMergeReplace(tag string) bool {
	return tag == "override" || tag == "reset"
}

func composeMergeFields(n *loader.Node) map[string]struct{} {
	out := map[string]struct{}{}
	if n == nil {
		return out
	}
	for _, e := range n.Map {
		if e.Value == nil || !composeMergeReplace(e.Value.Tag) {
			continue
		}
		field, ok := manifestFieldForComposeKey(e.Key)
		if !ok {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func manifestFieldForComposeKey(k string) (string, bool) {
	switch k {
	case "volumes":
		return "mounts", true
	case "depends_on":
		return "dependsOn", true
	case "deploy", "mem_limit":
		return "resources", true
	case "command", "entrypoint", "environment", "ports", "networks", "healthcheck", "restart":
		return k, true
	default:
		return "", false
	}
}

func mergeFragment(acc, frag serviceFragment) serviceFragment {
	if acc.body == nil {
		acc.body = map[string]any{}
	}
	if acc.fields == nil {
		acc.fields = map[string]struct{}{}
	}
	for field := range frag.fields {
		acc.fields[field] = struct{}{}
		if v, ok := frag.body[field]; ok {
			acc.body[field] = v
		}
	}
	return acc
}

func applyOverrideFields(dst map[string]any, frag serviceFragment) {
	if dst == nil {
		return
	}
	for field := range frag.fields {
		v, ok := frag.body[field]
		if !ok {
			continue
		}
		if manifestValueEmpty(v) {
			delete(dst, field)
			continue
		}
		dst[field] = v
	}
}

func manifestValueEmpty(v any) bool {
	switch t := v.(type) {
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}
