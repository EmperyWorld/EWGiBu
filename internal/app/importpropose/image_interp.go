package importpropose

import "strings"

// classifyImportImageRef turns a Compose image string into a literal pull ref
// or a parameter id. Nested ${VAR:-${…}} is peeled until the remainder is a
// literal image or a whole-value interpolation with no literal default.
func classifyImportImageRef(ref string) (literal, paramID string, ok bool) {
	cur := strings.TrimSpace(ref)
	if cur == "" {
		return "", "", false
	}
	for i := 0; i < 32; i++ {
		if !interpRe.MatchString(cur) {
			return cur, "", true
		}
		def, hasDef := composeSingleInterpDefault(cur)
		if hasDef {
			def = strings.TrimSpace(def)
			if def == "" || def == cur {
				break
			}
			cur = def
			continue
		}
		if id, bare := imageRefParameterID(cur); bare {
			return "", id, true
		}
		break
	}
	lit, tagOK := resolveImportImageTagRef(cur)
	if !tagOK {
		return "", "", false
	}
	return lit, "", true
}

func resolveImportImageTagRef(ref string) (literal string, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || !interpRe.MatchString(ref) {
		return ref, ref != ""
	}
	if strings.Contains(ref, "@") {
		return "", false
	}
	colon := lastImageRefTagColon(ref)
	if colon < 0 {
		return "", false
	}
	prefix := ref[:colon]
	tagPart := ref[colon+1:]
	if tagPart == "" || strings.Contains(tagPart, "/") {
		return "", false
	}
	if strings.Contains(prefix, "${") {
		return "", false
	}
	resolvedTag, ok := resolveImportImageTagPart(tagPart)
	if !ok || resolvedTag == "" || interpRe.MatchString(resolvedTag) {
		return "", false
	}
	return prefix + ":" + resolvedTag, true
}

func resolveImportImageTagPart(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	if !interpRe.MatchString(tag) {
		return tag, true
	}
	if lit, ok := resolveImportLiteralScalar(tag); ok && strings.TrimSpace(lit) != "" {
		return strings.TrimSpace(lit), true
	}
	return "", false
}

// lastImageRefTagColon returns the index of the colon before the image tag
// (last colon outside ${…} when the suffix contains no '/').
func lastImageRefTagColon(ref string) int {
	last := -1
	depth := 0
	for i := 0; i < len(ref); i++ {
		if i+1 < len(ref) && ref[i] == '$' && ref[i+1] == '{' {
			depth++
			i++
			continue
		}
		if depth > 0 {
			if ref[i] == '}' {
				depth--
			}
			continue
		}
		if ref[i] != ':' {
			continue
		}
		suffix := ref[i+1:]
		if suffix == "" || strings.Contains(suffix, "/") {
			continue
		}
		last = i
	}
	return last
}

// imageRefParameterID reports a whole-value Compose interpolation with no default:
// ${VAR} or ${VAR:?message}. The caller maps it to a parameter gap.
func imageRefParameterID(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if len(ref) < 4 || !strings.HasPrefix(ref, "${") || !strings.HasSuffix(ref, "}") {
		return "", false
	}
	if strings.Count(ref, "${") != 1 {
		return "", false
	}
	inner := ref[2 : len(ref)-1]
	if strings.Contains(inner, ":-") {
		return "", false
	}
	id := sanitizeID(paramIDFromInterp(ref))
	if id == "" {
		return "", false
	}
	return id, true
}

func anyMutablePullRef(images map[string]any) bool {
	for _, raw := range images {
		m, _ := raw.(map[string]any)
		switch ref := m["ref"].(type) {
		case string:
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			if !pullRefDigestPinned(ref) {
				return true
			}
		case map[string]any:
			if _, ok := ref["parameter"].(string); ok {
				return true
			}
		}
	}
	return false
}

func pullRefDigestPinned(ref string) bool {
	const marker = "@sha256:"
	i := strings.LastIndex(ref, marker)
	if i < 0 {
		return false
	}
	dig := ref[i+len(marker):]
	if len(dig) != 64 {
		return false
	}
	for _, c := range dig {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
