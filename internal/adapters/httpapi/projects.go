package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"iacpack/internal/app/projectinit"
	"iacpack/internal/policy"
)

const rootProjectID = "workspace"

var projectIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type projectInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Active bool   `json:"active"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": map[string]any{"projects": s.listProjects()},
	})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	id := strings.TrimSpace(req.Name)
	if !projectIDRe.MatchString(id) || id == rootProjectID {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "project name must match ^[a-z][a-z0-9-]{0,62}$", nil)
		return
	}
	dir := filepath.Join(s.studioRoot, "projects", id)
	if !underRoot(s.studioRoot, dir) {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "path escapes studio", nil)
		return
	}
	if _, err := os.Lstat(dir); err == nil {
		writeProblem(w, http.StatusConflict, "Conflict", "project directory already exists", nil)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
		return
	}
	body := strings.Replace(projectinit.MinimalYAML, "\n  name: example\n", "\n  name: "+id+"\n", 1)
	manifest := filepath.Join(dir, "iacpack.yaml")
	if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
		return
	}
	if err := s.activateProject(id); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":     true,
		"result": map[string]any{"id": id, "projects": s.listProjects()},
	})
}

func (s *Server) handleSelectProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	if err := s.activateProject(strings.TrimSpace(req.ID)); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": map[string]any{"projects": s.listProjects()},
	})
}

func (s *Server) listProjects() []projectInfo {
	active := s.workspace()
	var out []projectInfo
	if _, err := os.Lstat(filepath.Join(s.studioRoot, "iacpack.yaml")); err == nil {
		out = append(out, projectInfo{
			ID: rootProjectID, Name: manifestProjectName(filepath.Join(s.studioRoot, "iacpack.yaml")),
			Dir: "", Active: active == s.studioRoot,
		})
	}
	projectsDir := filepath.Join(s.studioRoot, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return out
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || !projectIDRe.MatchString(e.Name()) {
			continue
		}
		manifest := filepath.Join(projectsDir, e.Name(), "iacpack.yaml")
		if _, err := os.Lstat(manifest); err != nil {
			continue
		}
		ids = append(ids, e.Name())
	}
	sort.Strings(ids)
	for _, id := range ids {
		dir := filepath.Join(projectsDir, id)
		out = append(out, projectInfo{
			ID: id, Name: manifestProjectName(filepath.Join(dir, "iacpack.yaml")),
			Dir: filepath.ToSlash(filepath.Join("projects", id)), Active: active == dir,
		})
	}
	return out
}

func (s *Server) activateProject(id string) error {
	var dir string
	switch {
	case id == rootProjectID:
		dir = s.studioRoot
	case projectIDRe.MatchString(id):
		dir = filepath.Join(s.studioRoot, "projects", id)
	default:
		return errProjectID
	}
	if !underRoot(s.studioRoot, dir) {
		return errProjectID
	}
	manifest := filepath.Join(dir, "iacpack.yaml")
	if _, err := os.Lstat(manifest); err != nil {
		return err
	}
	pol := policy.MustDefault()
	if raw, err := os.ReadFile(filepath.Join(dir, "compile-policy.json")); err == nil {
		if parsed, err := policy.Parse(raw); err == nil {
			pol = parsed
		}
	}
	s.mu.Lock()
	s.cfg.Workspace = dir
	s.cfg.Policy = pol
	s.mu.Unlock()
	if s.watcher != nil {
		s.watcher.setPath(s.manifestAbs())
	}
	return nil
}

var errProjectID = errString("invalid project id")

type errString string

func (e errString) Error() string { return string(e) }

func manifestProjectName(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
	}
	return ""
}
