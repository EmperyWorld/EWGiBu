package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"iacpack/internal/app/secretfile"
)

func (s *Server) handleSecretGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Replace bool   `json:"replace"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && err != io.EOF {
		detail := "invalid json"
		if strings.Contains(err.Error(), "unknown field") {
			detail = "request must not include a secret value"
		}
		writeProblem(w, http.StatusBadRequest, "Bad Request", detail, nil)
		return
	}
	rel, err := secretfile.Write(s.workspace(), req.ID, secretfile.Kind(req.Kind), req.Replace)
	if err != nil {
		switch {
		case errors.Is(err, secretfile.ErrBadID), errors.Is(err, secretfile.ErrBadKind):
			writeProblem(w, http.StatusBadRequest, "Bad Request", err.Error(), nil)
		case errors.Is(err, secretfile.ErrExists):
			writeProblem(w, http.StatusConflict, "Conflict", "secret file already exists", nil)
		case errors.Is(err, secretfile.ErrSymlink):
			writeProblem(w, http.StatusBadRequest, "Bad Request", err.Error(), nil)
		default:
			writeProblem(w, http.StatusInternalServerError, "Internal Error", "could not write secret file", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"result": map[string]any{
			"id":       req.ID,
			"kind":     req.Kind,
			"path":     rel,
			"replaced": req.Replace,
		},
		"diagnostics": []any{},
	})
}
