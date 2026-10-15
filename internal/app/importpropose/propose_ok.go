package importpropose

import "iacpack/internal/diagnostic"

// proposeResultOK reports whether Accept may proceed (ADR-020 §7).
func proposeResultOK(diags []diagnostic.Diagnostic, hasDraftContent bool) bool {
	if !hasDraftContent {
		return false
	}
	return !diagnostic.HasError(diags)
}
