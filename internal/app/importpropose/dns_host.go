package importpropose

import "strings"

// rewriteContainerDNS replaces Compose container_name hosts in environment
// literals with the service id. container_name is not DNS on the compose network.
func rewriteContainerDNS(services map[string]any, aliases map[string]string) {
	if len(aliases) == 0 {
		return
	}
	for _, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		env, ok := svc["environment"].(map[string]any)
		if !ok {
			continue
		}
		for k, v := range env {
			env[k] = rewriteEnvHosts(v, aliases)
		}
	}
}

func rewriteEnvHosts(v any, aliases map[string]string) any {
	switch t := v.(type) {
	case string:
		return replaceHostAliases(t, aliases)
	case map[string]any:
		if lit, ok := t["literal"].(string); ok {
			t["literal"] = replaceHostAliases(lit, aliases)
			return t
		}
		if parts, ok := t["parts"].([]any); ok {
			for i, p := range parts {
				parts[i] = rewriteEnvHosts(p, aliases)
			}
			t["parts"] = parts
		}
		return t
	default:
		return v
	}
}

func replaceHostAliases(s string, aliases map[string]string) string {
	// Longer aliases first so stack1-seaweedfs is not partially rewritten.
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if len(names[j]) > len(names[i]) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for _, name := range names {
		s = replaceHost(s, name, aliases[name])
	}
	return s
}

func replaceHost(s, alias, svc string) string {
	if alias == "" || alias == svc {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], alias)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		after := j + len(alias)
		if hostBounded(s, j, after) {
			b.WriteString(s[i:j])
			b.WriteString(svc)
			i = after
			continue
		}
		b.WriteString(s[i : j+1])
		i = j + 1
	}
	return b.String()
}

func hostBounded(s string, start, end int) bool {
	if start > 0 {
		prev := s[:start]
		switch {
		case strings.HasSuffix(prev, "://"):
		case strings.HasSuffix(prev, "@"):
		default:
			if start > 0 && isHostChar(s[start-1]) {
				return false
			}
		}
	}
	if end < len(s) {
		c := s[end]
		if isHostChar(c) {
			return false
		}
	}
	return true
}

func isHostChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
}
