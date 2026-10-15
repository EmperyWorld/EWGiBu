package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/adapters/httpapi/uistatic"
	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/build"
	"iacpack/internal/app/graph"
	"iacpack/internal/app/importpropose"
	"iacpack/internal/app/inspect"
	applock "iacpack/internal/app/lock"
	"iacpack/internal/app/releasetemplates"
	"iacpack/internal/app/validate"
	"iacpack/internal/app/workspaceingest"
	"iacpack/internal/artifact"
	"iacpack/internal/buildinfo"
	"iacpack/internal/diagnostic"
	"iacpack/internal/discover"
	"iacpack/internal/inputs"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/policy"
	"iacpack/internal/schemaembed"
	openapiv1 "iacpack/openapi/v1"
)

const maxBody = 4 << 20        // 4 MiB
const maxUploadBody = 96 << 20 // workspace upload JSON (base64)

// Config is fixed at process start.
type Config struct {
	Workspace   string
	ManifestRel string // workspace-relative, default iacpack.yaml
	OutputRel   string // workspace-relative build output root
	Policy      policy.CompilePolicy
	Addr        string // must be loopback
}

// Server is the loopback authoring API.
type Server struct {
	mu         sync.RWMutex
	cfg        Config
	studioRoot string
	sessions   *sessionStore
	builds     *buildQueue
	hub        *eventHub
	watcher    *manifestWatcher
	stopWatch  context.CancelFunc
}

// New constructs a server. Addr host must be loopback.
func New(cfg Config) (*Server, error) {
	if cfg.ManifestRel == "" {
		cfg.ManifestRel = "iacpack.yaml"
	}
	if cfg.OutputRel == "" {
		cfg.OutputRel = ".iacpack-out"
	}
	if cfg.Policy.APIVersion == "" {
		cfg.Policy = policy.MustDefault()
	}
	if err := assertLoopbackAddr(cfg.Addr); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = abs
	if _, err := workspace.New(abs); err != nil {
		return nil, err
	}
	hub := newEventHub(1000)
	s := &Server{
		cfg: cfg, studioRoot: abs, sessions: newSessionStore(), builds: newBuildQueue(2), hub: hub,
	}
	s.watcher = newManifestWatcher(s.manifestAbs(), hub)
	watchCtx, cancel := context.WithCancel(context.Background())
	s.stopWatch = cancel
	go s.watcher.run(watchCtx.Done())
	return s, nil
}

// Handler returns the root HTTP handler.
func (s *Server) workspace() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Workspace
}

func (s *Server) policy() policy.CompilePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Policy
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/workspace", s.handleWorkspaceInfo)
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/v1/projects", s.withCSRF(s.handleCreateProject))
	mux.HandleFunc("POST /api/v1/projects/select", s.withCSRF(s.handleSelectProject))
	mux.HandleFunc("GET /api/v1/schema/manifest", s.handleSchema)
	mux.HandleFunc("GET /api/v1/manifest", s.handleGetManifest)
	mux.HandleFunc("PUT /api/v1/manifest", s.withCSRF(s.handlePutManifest))
	mux.HandleFunc("POST /api/v1/manifest/save-as", s.withCSRF(s.handleSaveAs))
	mux.HandleFunc("POST /api/v1/workspace/upload", s.withCSRF(s.handleWorkspaceUpload))
	mux.HandleFunc("GET /api/v1/workspace/files", s.handleWorkspaceFiles)
	mux.HandleFunc("POST /api/v1/export/manifest", s.withCSRF(s.handleExport))
	mux.HandleFunc("POST /api/v1/validate", s.withCSRF(s.handleValidate))
	mux.HandleFunc("POST /api/v1/inspect/graph", s.withCSRF(s.handleGraph))
	mux.HandleFunc("POST /api/v1/preview/compose", s.withCSRF(s.handlePreviewCompose))
	mux.HandleFunc("POST /api/v1/preview/artifacts", s.withCSRF(s.handlePreviewArtifacts))
	mux.HandleFunc("POST /api/v1/lock", s.withCSRF(s.handleLock))
	mux.HandleFunc("POST /api/v1/builds", s.withCSRF(s.handleStartBuild))
	mux.HandleFunc("GET /api/v1/builds/{id}", s.handleGetBuild)
	mux.HandleFunc("DELETE /api/v1/builds/{id}", s.withCSRF(s.handleCancelBuild))
	mux.HandleFunc("GET /api/v1/builds/{id}/events", s.handleBuildEvents)
	mux.HandleFunc("GET /api/v1/manifest/events", s.handleManifestEvents)
	mux.HandleFunc("GET /api/v1/inputs", s.handleInputs)
	mux.HandleFunc("GET /api/v1/discover", s.handleDiscover)
	mux.HandleFunc("POST /api/v1/import/propose", s.withCSRF(s.handleImportPropose))
	mux.HandleFunc("POST /api/v1/import/onboard", s.withCSRF(s.handleImportApplyStubs))
	mux.HandleFunc("POST /api/v1/import/env-example", s.withCSRF(s.handleImportEnvExample))
	mux.HandleFunc("POST /api/v1/import/stub-preview", s.withCSRF(s.handleImportStubPreview))
	mux.HandleFunc("POST /api/v1/import/prepare", s.withCSRF(s.handleImportPrepare))
	mux.HandleFunc("POST /api/v1/import/write-draft", s.withCSRF(s.handleImportWriteDraft))
	mux.HandleFunc("POST /api/v1/import/resource-stubs", s.withCSRF(s.handleImportResourceStubs))
	mux.HandleFunc("POST /api/v1/release/templates/propose", s.withCSRF(s.handleReleaseTemplatesPropose))
	mux.HandleFunc("POST /api/v1/release/templates/write", s.withCSRF(s.handleReleaseTemplatesWrite))
	mux.HandleFunc("POST /api/v1/secrets/generate", s.withCSRF(s.handleSecretGenerate))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	mux.Handle("GET /assets/", s.staticAssets())
	return s.middleware(mux)
}

// RoutePatterns returns the HTTP method+path patterns registered by Handler.
// Used by OpenAPI drift tests.
func RoutePatterns() []string {
	return []string{
		"GET /api/v1/health",
		"GET /api/v1/workspace",
		"GET /api/v1/projects",
		"POST /api/v1/projects",
		"POST /api/v1/projects/select",
		"GET /api/v1/schema/manifest",
		"GET /api/v1/manifest",
		"PUT /api/v1/manifest",
		"POST /api/v1/manifest/save-as",
		"POST /api/v1/workspace/upload",
		"GET /api/v1/workspace/files",
		"POST /api/v1/export/manifest",
		"POST /api/v1/validate",
		"POST /api/v1/inspect/graph",
		"POST /api/v1/preview/compose",
		"POST /api/v1/preview/artifacts",
		"POST /api/v1/lock",
		"POST /api/v1/builds",
		"GET /api/v1/builds/{id}",
		"DELETE /api/v1/builds/{id}",
		"GET /api/v1/builds/{id}/events",
		"GET /api/v1/manifest/events",
		"GET /api/v1/inputs",
		"GET /api/v1/discover",
		"POST /api/v1/import/propose",
		"POST /api/v1/import/onboard",
		"POST /api/v1/import/env-example",
		"POST /api/v1/import/stub-preview",
		"POST /api/v1/import/prepare",
		"POST /api/v1/import/write-draft",
		"POST /api/v1/import/resource-stubs",
		"POST /api/v1/release/templates/propose",
		"POST /api/v1/release/templates/write",
		"POST /api/v1/secrets/generate",
	}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		// CORS disabled: never emit ACAO. Reject cross-origin browser requests.
		if origin := r.Header.Get("Origin"); origin != "" && !allowedOrigin(origin, r.Host) {
			writeProblem(w, http.StatusForbidden, "Forbidden", "cross-origin requests are not allowed", nil)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodDelete {
			ct := r.Header.Get("Content-Type")
			if ct != "" && !strings.HasPrefix(ct, "application/json") &&
				!strings.HasPrefix(ct, "application/yaml") && !strings.HasPrefix(ct, "text/yaml") &&
				!strings.HasPrefix(ct, "application/x-yaml") {
				writeProblem(w, http.StatusUnsupportedMediaType, "Unsupported Media Type", "unsupported content-type", nil)
				return
			}
		}
		limit := int64(maxBody)
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspace/upload" {
			limit = maxUploadBody
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin, host string) bool {
	// Same-origin loopback only.
	if origin == "null" {
		return false
	}
	uHost := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	if i := strings.IndexByte(uHost, '/'); i >= 0 {
		uHost = uHost[:i]
	}
	return strings.EqualFold(uHost, host)
}

func (s *Server) withCSRF(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := readSessionID(r)
		csrf := r.Header.Get(csrfHeader)
		if sid == "" || csrf == "" || !s.sessions.valid(sid, csrf) {
			writeProblem(w, http.StatusForbidden, "Forbidden", "missing or invalid CSRF/session", nil)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "product": buildinfo.Product, "version": buildinfo.Version,
	})
}

func (s *Server) handleWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace":  s.workspace(),
		"manifest":   s.cfg.ManifestRel,
		"outputRoot": s.cfg.OutputRel,
	})
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(schemaembed.ManifestV1Alpha1())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	id, csrf, err := s.sessions.create()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", "session bootstrap failed", nil)
		return
	}
	setSessionCookie(w, id)
	raw, err := uistatic.Dist.ReadFile("dist/index.html")
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", "UI index missing", nil)
		return
	}
	html := strings.ReplaceAll(string(raw), "__IACPACK_CSRF__", csrf)
	// Deterministic Vite names (assets/index.js) must not be cached forever;
	// stamp content digests so browsers pick up rebuilt embeds.
	html = stampAssetQuery(html, "assets/index.js")
	html = stampAssetQuery(html, "assets/index.css")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, html)
}

func stampAssetQuery(html, rel string) string {
	b, err := uistatic.Dist.ReadFile("dist/" + rel)
	if err != nil {
		return html
	}
	sum := sha256.Sum256(b)
	v := hex.EncodeToString(sum[:8])
	old := `"/` + rel + `"`
	neu := `"/` + rel + `?v=` + v + `"`
	return strings.ReplaceAll(html, old, neu)
}

func (s *Server) staticAssets() http.Handler {
	sub, err := fs.Sub(uistatic.Dist, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", "UI assets missing", nil)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unhashed deterministic names (vite.config.ts) — revalidate, do not mark immutable.
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openapiv1.YAML)
}

func (s *Server) manifestAbs() string {
	return filepath.Join(s.workspace(), filepath.FromSlash(s.cfg.ManifestRel))
}

func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	path := s.manifestAbs()
	raw, err := os.ReadFile(path)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "manifest missing", nil)
		return
	}
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(raw)
}

func (s *Server) handlePutManifest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "read body failed", nil)
		return
	}
	path := s.manifestAbs()
	_, statErr := os.Lstat(path)
	exists := statErr == nil
	ifm := r.Header.Get("If-Match")
	ifnm := r.Header.Get("If-None-Match")
	if !exists {
		if ifnm != "*" {
			writeProblem(w, http.StatusNotFound, "Not Found", "manifest missing; use If-None-Match: * to create", nil)
			return
		}
		afs := filesystem.OS{}
		if err := afs.WriteFileAtomic(path, body, 0o644); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
			return
		}
	} else {
		cur, err := os.ReadFile(path)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
			return
		}
		sum := sha256.Sum256(cur)
		want := `"` + hex.EncodeToString(sum[:]) + `"`
		if ifm == "" || ifm != want {
			writeProblem(w, http.StatusConflict, "Conflict", "If-Match does not match current revision", nil)
			return
		}
		afs := filesystem.OS{}
		if err := afs.WriteFileAtomicReplace(path, body, 0o644); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
			return
		}
	}
	sum := sha256.Sum256(body)
	rev := hex.EncodeToString(sum[:])
	etag := `"` + rev + `"`
	if s.watcher != nil {
		s.watcher.noteSelfWrite(rev)
		// Align watcher baseline so we don't emit after debounce of self write.
		s.watcher.mu.Lock()
		s.watcher.last = fileSnapshot{exists: true, permOK: true, rev: rev, rawOK: true, valid: looksLikeProjectYAML(body)}
		s.watcher.pending = nil
		s.watcher.mu.Unlock()
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, map[string]any{
		"path": s.cfg.ManifestRel, "etag": etag, "revision": rev,
	})
}

func (s *Server) handleWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	dir, entries, diags := workspaceingest.ListDir(s.workspace(), r.URL.Query().Get("dir"))
	if entries == nil {
		entries = []workspaceingest.DirEntry{}
	}
	status := http.StatusOK
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{
		"ok":          ok,
		"result":      map[string]any{"dir": dir, "entries": entries},
		"diagnostics": diagnostic.ToMaps(diags),
	})
}

func (s *Server) handleWorkspaceUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode          string `json:"mode"`
		Target        string `json:"target"`
		Overwrite     bool   `json:"overwrite"`
		ContentBase64 string `json:"contentBase64"`
		Files         []struct {
			Path          string `json:"path"`
			ContentBase64 string `json:"contentBase64"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	ingestReq := workspaceingest.Request{
		Mode:      req.Mode,
		Target:    req.Target,
		Overwrite: req.Overwrite,
	}
	if req.ContentBase64 != "" {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.ContentBase64))
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid contentBase64", nil)
			return
		}
		ingestReq.File = raw
	}
	if len(req.Files) > 0 {
		apiParts := make([]workspaceingest.UploadFilePart, len(req.Files))
		for i, f := range req.Files {
			apiParts[i] = workspaceingest.UploadFilePart{Path: f.Path, ContentBase64: f.ContentBase64}
		}
		parts, pdiags := workspaceingest.DecodeBase64Files(apiParts)
		if diagnostic.HasError(pdiags) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"ok": false, "result": map[string]any{}, "diagnostics": diagnostic.ToMaps(pdiags),
			})
			return
		}
		ingestReq.Tree = parts
	}
	res, diags := workspaceingest.Ingest(s.workspace(), ingestReq, workspaceingest.DefaultLimits())
	status := http.StatusOK
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{
		"ok":          ok,
		"result":      res.MapForAPI(),
		"diagnostics": diagnostic.ToMaps(diags),
	})
}

func (s *Server) handleSaveAs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	rel, err := containWorkspacePath(req.Path)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", err.Error(), nil)
		return
	}
	abs := filepath.Join(s.workspace(), filepath.FromSlash(rel))
	if !underRoot(s.workspace(), abs) {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "path escapes workspace", nil)
		return
	}
	if _, err := os.Lstat(abs); err == nil {
		writeProblem(w, http.StatusConflict, "Conflict", "refusing to overwrite existing file", nil)
		return
	}
	base := strings.ToLower(filepath.Base(rel))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "refusing to write populated env files; use .env.example placeholders only", nil)
		return
	}
	afs := filesystem.OS{}
	if err := afs.WriteFileAtomic(abs, []byte(req.Content), 0o644); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
		return
	}
	sum := sha256.Sum256([]byte(req.Content))
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	writeJSON(w, http.StatusCreated, map[string]any{
		"path": rel, "etag": etag, "revision": hex.EncodeToString(sum[:]),
	})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	fs := filesystem.OS{}
	res := validate.RunWithOptions(fs, s.manifestAbs(), validate.Options{
		Policy: s.policy(), Workspace: mustWS(s.workspace()),
	})
	if !res.OK {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid Manifest", "validation failed", res.Diagnostics)
		return
	}
	canon, err := v1alpha1.CanonicalSemantic(res.Project)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(canon)
	_, _ = w.Write([]byte("\n"))
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	set, values := parseValuesBody(r)
	fs := filesystem.OS{}
	res := validate.RunWithOptions(fs, s.manifestAbs(), validate.Options{
		Policy: s.policy(), Set: set, Values: values, Workspace: mustWS(s.workspace()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": res.OK, "result": map[string]any{"syntaxOnly": res.SyntaxOnly}, "diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	fs := filesystem.OS{}
	res := graph.Run(fs, s.manifestAbs(), validate.Options{
		Policy: s.policy(), Workspace: mustWS(s.workspace()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": res.OK, "result": res.Model, "diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handlePreviewCompose(w http.ResponseWriter, r *http.Request) {
	set, values := parseValuesBody(r)
	fs := filesystem.OS{}
	cw := &captureWriter{}
	res := build.Run(fs, cw, s.manifestAbs(), build.Options{
		Policy: s.policy(), Set: set, Values: values, Workspace: mustWS(s.workspace()),
		Output: filepath.Join(s.workspace(), s.cfg.OutputRel, "preview-compose"),
		Format: "directory",
	})
	preview := ""
	if res.OK && cw.tree != nil {
		if e, ok := cw.tree.Get("rendered/compose.preview.yaml"); ok {
			preview = string(e.Content)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          res.OK,
		"result":      map[string]any{"compose": preview, "files": res.TreePaths, "hasPreview": res.Preview},
		"diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	fs := filesystem.OS{}
	res := applock.Run(fs, s.manifestAbs(), applock.Options{
		Policy: s.policy(), Workspace: mustWS(s.workspace()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": res.OK, "result": map[string]any{"path": res.LockPath}, "diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handlePreviewArtifacts(w http.ResponseWriter, r *http.Request) {
	fs := filesystem.OS{}
	cw := &captureWriter{}
	res := build.Run(fs, cw, s.manifestAbs(), build.Options{
		Policy: s.policy(), Workspace: mustWS(s.workspace()),
		Output: filepath.Join(s.workspace(), s.cfg.OutputRel, "preview-artifacts"),
		Format: "directory",
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          res.OK,
		"result":      map[string]any{"files": res.TreePaths},
		"diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleInputs(w http.ResponseWriter, r *http.Request) {
	fs := filesystem.OS{}
	res := inspect.RunInputs(fs, s.manifestAbs(), validate.Options{
		Policy: s.policy(), Workspace: mustWS(s.workspace()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": res.OK, "result": res.Model, "diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	ws := mustWS(s.workspace())
	if ws == nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", "workspace unavailable", nil)
		return
	}
	sugs, err := discover.Run(ws)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", "discover failed", nil)
		return
	}
	_, model := inspect.FormatDiscover(sugs)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "result": model, "diagnostics": []any{},
	})
}

func (s *Server) handleImportPropose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Compose         string         `json:"compose"`
		Composes        []string       `json:"composes"`
		Dockerfiles     []string       `json:"dockerfiles"`
		Name            string         `json:"name"`
		ActiveProfiles  []string       `json:"activeProfiles"`
		Strict          bool           `json:"strict"`
		ExistingProject map[string]any `json:"existingProject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace:       s.workspace(),
		Compose:         req.Compose,
		Composes:        req.Composes,
		Dockerfiles:     req.Dockerfiles,
		Name:            req.Name,
		ActiveProfiles:  req.ActiveProfiles,
		Strict:          req.Strict,
		ExistingProject: req.ExistingProject,
	})
	status := http.StatusOK
	if !res.OK {
		status = http.StatusUnprocessableEntity
	}
	refusals := res.Refusals
	if refusals == nil {
		refusals = []importpropose.Refusal{}
	}
	gaps := res.ParameterGaps
	if gaps == nil {
		gaps = []importpropose.ParameterGap{}
	}
	sugs := res.SecretSuggestions
	if sugs == nil {
		sugs = []importpropose.SecretSuggestion{}
	}
	writeJSON(w, status, map[string]any{
		"ok": res.OK,
		"result": map[string]any{
			"project":           res.Project,
			"summary":           res.Summary,
			"sources":           res.Sources,
			"refusals":          refusals,
			"parameterGaps":     gaps,
			"secretSuggestions": sugs,
			"envExample":        res.EnvExample,
		},
		"diagnostics": diagnostic.ToMaps(importpropose.DiagnosticsForAPI(res.Diagnostics, refusals)),
	})
}

func (s *Server) handleImportApplyStubs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project         map[string]any                   `json:"project"`
		SecretIDs       []string                         `json:"secretIds"`
		Suggestions     []importpropose.SecretSuggestion `json:"suggestions"`
		ParamDefaults   map[string]any                   `json:"paramDefaults"`
		AttachMounts    bool                             `json:"attachMounts"`
		WriteEnvExample bool                             `json:"writeEnvExample"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	applied, err := importpropose.ApplySuggestions(req.Project, importpropose.ApplyOptions{
		SecretIDs:     req.SecretIDs,
		Suggestions:   req.Suggestions,
		ParamDefaults: req.ParamDefaults,
		AttachMounts:  req.AttachMounts,
	})
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error(), nil)
		return
	}
	example := importpropose.EnvExamplePlaceholders(applied, req.SecretIDs)
	if req.WriteEnvExample {
		if err := writeEnvExampleFile(s.workspace(), example); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"result": map[string]any{
			"project":    applied,
			"envExample": example,
			"wrotePath":  ternary(req.WriteEnvExample, ".env.example", ""),
		},
		"diagnostics": []any{},
	})
}

func (s *Server) handleImportPrepare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project map[string]any `json:"project"`
		Path    string         `json:"path"`
		Replace bool           `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	if req.Project == nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "project is required", nil)
		return
	}
	res, diags := importpropose.PrepareImportWorkspace(s.workspace(), req.Project, req.Path, req.Replace)
	status := http.StatusOK
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{
		"ok":          ok,
		"result":      res.MapForAPI(),
		"diagnostics": diagnostic.ToMaps(diags),
	})
}

func (s *Server) handleImportWriteDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project map[string]any `json:"project"`
		Path    string         `json:"path"`
		Replace bool           `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	if req.Project == nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "project is required", nil)
		return
	}
	written, diags := importpropose.WriteImportDraft(s.workspace(), req.Path, req.Project, req.Replace)
	status := http.StatusCreated
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
		for _, d := range diags {
			if d.Code == diagnostic.CodeImportCollision {
				status = http.StatusConflict
				break
			}
		}
	}
	writeJSON(w, status, map[string]any{
		"ok":          ok,
		"result":      importpropose.WriteDraftResult{Path: written}.MapForAPI(),
		"diagnostics": diagnostic.ToMaps(diags),
	})
}

func (s *Server) handleImportStubPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project     map[string]any             `json:"project"`
		BindChoices []importpropose.BindChoice `json:"bindChoices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	if req.Project == nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "project is required", nil)
		return
	}
	project := req.Project
	var diags []diagnostic.Diagnostic
	if len(req.BindChoices) > 0 {
		project, diags = importpropose.ApplyBindChoices(s.workspace(), project, req.BindChoices)
	}
	plan, planDiags := importpropose.PlanImportStubs(s.workspace(), project)
	diags = append(diags, planDiags...)
	pathDiags := importpropose.MissingResourceSourceDiagsForAPI(s.workspace(), project)
	status := http.StatusOK
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	result := plan.MapForAPI()
	result["project"] = project
	writeJSON(w, status, map[string]any{
		"ok":              ok,
		"result":          result,
		"diagnostics":     diagnostic.ToMaps(diags),
		"pathDiagnostics": diagnostic.ToMaps(pathDiags),
	})
}

func (s *Server) handleImportResourceStubs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project map[string]any `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	if req.Project == nil {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "project is required", nil)
		return
	}
	stubs, diags := importpropose.EnsureImportStubs(s.workspace(), req.Project)
	status := http.StatusOK
	ok := !diagnostic.HasError(diags)
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{
		"ok":          ok,
		"result":      stubs.MapForAPI(),
		"diagnostics": diagnostic.ToMaps(diags),
	})
}

func (s *Server) handleImportEnvExample(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project   map[string]any `json:"project"`
		SecretIDs []string       `json:"secretIds"`
		Write     bool           `json:"write"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	example := importpropose.EnvExamplePlaceholders(req.Project, req.SecretIDs)
	wrote := ""
	if req.Write {
		if err := writeEnvExampleFile(s.workspace(), example); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Error", err.Error(), nil)
			return
		}
		wrote = ".env.example"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"result": map[string]any{
			"envExample": example,
			"wrotePath":  wrote,
		},
		"diagnostics": []any{},
	})
}

func writeEnvExampleFile(workspace, content string) error {
	if containsEnvCanary(content) {
		return fmt.Errorf("refusing to write .env.example: content looks like a secret value")
	}
	abs := filepath.Join(workspace, ".env.example")
	afs := filesystem.OS{}
	return afs.WriteFileAtomic(abs, []byte(content), 0o644)
}

func containsEnvCanary(s string) bool {
	upper := strings.ToUpper(s)
	return strings.Contains(upper, "SECRET_CANARY") ||
		strings.Contains(upper, "SUPER_SECRET_CANARY") ||
		strings.Contains(upper, "CANARY_SECRET")
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func (s *Server) handleReleaseTemplatesPropose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	res := releasetemplates.Propose(releasetemplates.Request{
		Workspace: s.workspace(),
		Force:     req.Force,
	})
	status := http.StatusOK
	if !res.OK {
		status = http.StatusUnprocessableEntity
	}
	files := res.Files
	if files == nil {
		files = []releasetemplates.FilePlan{}
	}
	writeJSON(w, status, map[string]any{
		"ok": res.OK,
		"result": map[string]any{
			"force": res.Force,
			"wrote": false,
			"files": files,
		},
		"diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleReleaseTemplatesWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid json", nil)
		return
	}
	res := releasetemplates.Write(releasetemplates.Request{
		Workspace: s.workspace(),
		Force:     req.Force,
	})
	status := http.StatusOK
	if !res.OK {
		status = http.StatusUnprocessableEntity
	}
	files := res.Files
	if files == nil {
		files = []releasetemplates.FilePlan{}
	}
	writeJSON(w, status, map[string]any{
		"ok": res.OK,
		"result": map[string]any{
			"force": res.Force,
			"wrote": res.Wrote,
			"files": files,
		},
		"diagnostics": diagnostic.ToMaps(res.Diagnostics),
	})
}

func (s *Server) handleManifestEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "Internal Error", "streaming unsupported", nil)
		return
	}
	lastID := r.Header.Get("Last-Event-ID")
	// Initial handshake when no resume cursor.
	if lastID == "" {
		fmt.Fprint(w, encodeManifestSSE(ManifestEvent{Kind: EventResyncRequired, Revision: "bootstrap", ID: "bootstrap"}))
		flusher.Flush()
	}

	client := s.hub.subscribe(lastID)
	defer s.hub.unsubscribe(client)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-client.ch:
			if !open {
				return
			}
			if _, err := io.WriteString(w, encodeManifestSSE(ev)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// captureWriter materializes builds into memory only (no host write).
type captureWriter struct {
	tree *artifact.Tree
}

func (c *captureWriter) Exists(string) (bool, error) { return false, nil }
func (c *captureWriter) MaterializeNewDir(_ string, tree *artifact.Tree) error {
	c.tree = tree
	return nil
}

func parseValuesBody(r *http.Request) (map[string]string, map[string]any) {
	var body struct {
		Set    map[string]string `json:"set"`
		Values map[string]any    `json:"values"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Set == nil {
		body.Set = map[string]string{}
	}
	return body.Set, body.Values
}

func mustWS(root string) inputs.FS {
	ws, err := workspace.New(root)
	if err != nil {
		return nil
	}
	return ws
}

func containWorkspacePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	errRel := fmt.Errorf("path must be a relative workspace path without parent segments")
	if p == "" {
		return "", errRel
	}
	// Reject absolute, Windows separators, encoded dots, and NUL.
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || strings.Contains(p, "\x00") {
		return "", errRel
	}
	if strings.Contains(p, "%2e") || strings.Contains(p, "%2E") || strings.Contains(p, "%2f") || strings.Contains(p, "%2F") {
		return "", errRel
	}
	if err := inputs.ValidateRelPath(filepath.ToSlash(p)); err != nil {
		return "", errRel
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("path escapes workspace")
	}
	return clean, nil
}

func underRoot(root, abs string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(abs))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func assertLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("empty listen address")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("listen address must be a loopback IP, got %q", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("refusing non-loopback bind %q", host)
	}
	return nil
}

// ListenAndServe runs until context cancel.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout left unset so SSE can stream; request handlers bound work themselves.
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		if s.stopWatch != nil {
			s.stopWatch()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if s.stopWatch != nil {
			s.stopWatch()
		}
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
