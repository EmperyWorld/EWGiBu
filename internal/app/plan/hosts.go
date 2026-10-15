package plan

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
)

// unknownServiceHosts reports single-label hosts in environment values that are
// not service names. Dotted names are external and are left alone.
func unknownServiceHosts(dep ir.Deployment) []diagnostic.Diagnostic {
	known := map[string]struct{}{"localhost": {}}
	names := make([]string, 0, len(dep.Services))
	for id := range dep.Services {
		known[id] = struct{}{}
		names = append(names, id)
	}
	sort.Strings(names)
	var diags []diagnostic.Diagnostic
	for id, svc := range dep.Services {
		keys := make([]string, 0, len(svc.Environment))
		for k := range svc.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, host := range literalHosts(svc.Environment[k]) {
				if _, ok := known[host]; ok || strings.Contains(host, ".") || net.ParseIP(host) != nil {
					continue
				}
				diags = append(diags, diagnostic.Error("PLAN.UNKNOWN_HOST",
					fmt.Sprintf("service %s env %s=%q uses host %q; known services: %s",
						id, k, hostLiteral(svc.Environment[k]), host, strings.Join(names, ", "))))
			}
		}
	}
	return diags
}

func hostLiteral(vs ir.ValueSource) string {
	if s, ok := vs.Literal.(string); ok {
		return s
	}
	return ""
}

func literalHosts(vs ir.ValueSource) []string {
	if len(vs.Parts) > 0 {
		var out []string
		for _, p := range vs.Parts {
			out = append(out, literalHosts(p)...)
		}
		return out
	}
	s, ok := vs.Literal.(string)
	if !ok || s == "" {
		return nil
	}
	var hosts []string
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		host := u.Hostname()
		if host != "" {
			hosts = append(hosts, host)
		}
		return hosts
	}
	// host:port without a scheme
	if i := strings.IndexByte(s, ':'); i > 0 && !strings.Contains(s, "://") && !strings.Contains(s, "/") {
		hosts = append(hosts, s[:i])
	}
	return hosts
}
