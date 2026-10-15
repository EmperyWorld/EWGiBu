package importpropose

import (
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

type importOptions struct {
	activeProfiles map[string]struct{}
	strict         bool
}

func importOptionsFromRequest(req Request) importOptions {
	return importOptions{
		activeProfiles: activeProfileSet(req.ActiveProfiles),
		strict:         req.Strict,
	}
}

func normalizeActiveProfiles(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func activeProfileSet(profiles []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, p := range normalizeActiveProfiles(profiles) {
		set[p] = struct{}{}
	}
	return set
}

func composeServiceProfiles(n *loader.Node) []string {
	if n == nil {
		return nil
	}
	p := n.GetMap("profiles")
	if p == nil {
		return nil
	}
	switch p.Kind {
	case loader.KindString:
		s := strings.TrimSpace(p.Str)
		if s == "" {
			return nil
		}
		return []string{s}
	case loader.KindSeq:
		var out []string
		for _, c := range p.Seq {
			if c == nil || c.Kind != loader.KindString {
				continue
			}
			s := strings.TrimSpace(c.Str)
			if s != "" {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	default:
		return nil
	}
}

func serviceProfilesActive(active map[string]struct{}, serviceProfiles []string) bool {
	if len(serviceProfiles) == 0 {
		return true
	}
	if len(active) == 0 {
		return false
	}
	for _, p := range serviceProfiles {
		if _, ok := active[p]; ok {
			return true
		}
	}
	return false
}

func profileSkippedDiagnostic(
	sid, file, pointer string,
	serviceProfiles []string,
	line, col int,
) diagnostic.Diagnostic {
	msg := fmt.Sprintf(
		"service %s: skipped (Compose profiles %v inactive; pass --profile or activeProfiles like COMPOSE_PROFILES)",
		sid, serviceProfiles,
	)
	return diagnostic.Info(diagnostic.CodeImportProfileSkipped, msg).
		WithSource(file, pointer, line, col)
}
