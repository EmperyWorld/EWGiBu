package httpapi

import (
	"encoding/json"
	"net/http"

	"iacpack/internal/diagnostic"
)

// Problem is RFC 9457 Problem Details with diagnostics extension.
type Problem struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Status      int    `json:"status"`
	Detail      string `json:"detail,omitempty"`
	Diagnostics []any  `json:"diagnostics,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, title, detail string, diags []diagnostic.Diagnostic) {
	diagnostic.SortDeterministic(diags)
	p := Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	if len(diags) > 0 {
		p.Diagnostics = diagnostic.ToMaps(diags)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
