package importpropose

import (
	"iacpack/internal/diagnostic"
)

// DiagnosticsForAPI omits warning-level diagnostics already represented in refusals (UI/HTTP dedupe).
func DiagnosticsForAPI(diags []diagnostic.Diagnostic, refusals []Refusal) []diagnostic.Diagnostic {
	if len(diags) == 0 || len(refusals) == 0 {
		return diags
	}
	type key struct{ code, msg string }
	seen := map[key]struct{}{}
	for _, r := range refusals {
		seen[key{r.Code, r.Message}] = struct{}{}
	}
	out := make([]diagnostic.Diagnostic, 0, len(diags))
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityWarning {
			if _, dup := seen[key{d.Code, d.Message}]; dup {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}
