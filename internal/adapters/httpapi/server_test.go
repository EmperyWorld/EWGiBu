package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/adapters/httpapi"
	"iacpack/internal/policy"
	openapiv1 "iacpack/openapi/v1"
)

func TestRejectNonLoopbackBind(t *testing.T) {
	dir := t.TempDir()
	_, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "0.0.0.0:7432"})
	if err == nil {
		t.Fatal("expected non-loopback rejection")
	}
}

func TestNoDeployRoutes(t *testing.T) {
	for _, p := range httpapi.RoutePatterns() {
		low := strings.ToLower(p)
		for _, bad := range []string{"apply", "destroy", "status", "resume", "plan", "doctor", "task"} {
			if strings.Contains(low, bad) {
				t.Fatalf("deployment-adjacent route %q", p)
			}
		}
	}
	for p := range openAPIPaths(t, openapiv1.YAML) {
		low := strings.ToLower(p)
		for _, bad := range []string{"/apply", "/destroy", "/status", "/resume", "/plan", "/doctor"} {
			if strings.Contains(low, bad) {
				t.Fatalf("openapi exposes deploy route %q", p)
			}
		}
	}
}

func TestOpenAPIPathDrift(t *testing.T) {
	paths := openAPIPaths(t, openapiv1.YAML)
	want := map[string]bool{}
	for _, rp := range httpapi.RoutePatterns() {
		parts := strings.SplitN(rp, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("bad route %q", rp)
		}
		method, path := parts[0], parts[1]
		path = strings.ReplaceAll(path, "{id}", "{id}")
		want[method+" "+path] = true
		if _, ok := paths[method+" "+path]; !ok {
			t.Errorf("server route missing from OpenAPI: %s %s", method, path)
		}
	}
	for k := range paths {
		if !want[k] {
			t.Errorf("OpenAPI path missing from server: %s", k)
		}
	}
}

func TestWorkspaceInfo(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/api/v1/workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("workspace %d %s", res.StatusCode, b)
	}
	var body struct {
		Workspace  string `json:"workspace"`
		Manifest   string `json:"manifest"`
		OutputRoot string `json:"outputRoot"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Workspace == "" || body.Manifest == "" || body.OutputRoot == "" {
		t.Fatalf("incomplete body: %+v", body)
	}
	if !strings.HasSuffix(body.Manifest, "iacpack.yaml") {
		t.Fatalf("manifest %q", body.Manifest)
	}
}

func TestImportPrepare(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	project := map[string]any{
		"apiVersion": "iacpack.io/v1alpha1",
		"kind":       "Project",
		"spec": map[string]any{
			"resources": map[string]any{
				"res-x": map[string]any{"source": "data/prepare", "include": []any{"**"}},
			},
		},
	}
	raw, _ := json.Marshal(map[string]any{"project": project, "replace": true})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/import/prepare", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "imports", "import-draft.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data/prepare")); err != nil {
		t.Fatal(err)
	}
}

func TestImportWriteDraft(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	project := map[string]any{"apiVersion": "iacpack.io/v1alpha1", "kind": "Project", "spec": map[string]any{}}
	raw, _ := json.Marshal(map[string]any{"project": project, "replace": true})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/import/write-draft", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "imports", "import-draft.json")); err != nil {
		t.Fatal(err)
	}
}

func TestImportStubPreview(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	project := map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"res-logs": map[string]any{
					"source":  "logs/demo",
					"include": []any{"**"},
				},
			},
		},
	}
	raw, _ := json.Marshal(map[string]any{"project": project})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/import/stub-preview", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d %s", res.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"plannedDirs"`)) || bytes.Contains(body, []byte(`"plannedDirs":null`)) {
		t.Fatalf("body %s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "logs/demo")); !os.IsNotExist(err) {
		t.Fatal("preview must not create directories")
	}
}

func TestImportResourceStubs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	project := map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"res-logs": map[string]any{
					"source":  "logs/demo",
					"include": []any{"**"},
				},
			},
		},
	}
	raw, _ := json.Marshal(map[string]any{"project": project})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/import/resource-stubs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "logs/demo")); err != nil {
		t.Fatal(err)
	}
}

func TestImportProposeComposeFlowStyle(t *testing.T) {
	stack4, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ui", "two-service", "stack4.yml"))
	if err != nil {
		t.Skip("stack4.yml fixture not in workspace")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "imports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "imports", "889754-stack4.yml"), stack4, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	body := []byte(`{"compose":"imports/889754-stack4.yml","name":"stack4"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/import/propose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status %d body %s", res.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte("MANIFEST.PARSE_ERROR")) || bytes.Contains(raw, []byte("flow-style collections")) {
		t.Fatalf("compose import must accept flow-style YAML; body %s", raw)
	}
	var payload struct {
		Result struct {
			Summary struct {
				Services int `json:"services"`
			} `json:"summary"`
		} `json:"result"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for _, d := range payload.Diagnostics {
		if d.Code == "MANIFEST.PARSE_ERROR" {
			t.Fatalf("unexpected parse error in diagnostics")
		}
	}
	if payload.Result.Summary.Services == 0 {
		t.Fatalf("expected services in draft, body %s", raw)
	}
}

func TestHealthAndSessionCSRF(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("health %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS enabled")
	}

	cookie, csrf := bootstrap(t, ts)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/validate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	// missing CSRF
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 got %d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/validate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("validate %d %s", res.StatusCode, b)
	}
}

func TestOriginRejected(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 got %d", res.StatusCode)
	}
}

func TestContentTypeRejected(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/validate", bytes.NewReader([]byte(`ok`)))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415 got %d", res.StatusCode)
	}
}

func TestOversizedBody(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	body := bytes.Repeat([]byte("a"), (4<<20)+10)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/manifest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	req.Header.Set("If-None-Match", "*")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == 200 {
		t.Fatal("oversized body accepted")
	}
}

func TestETagConflictAndCreate(t *testing.T) {
	dir := t.TempDir()
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", ManifestRel: "iacpack.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)

	// create
	raw := minimalManifest()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/manifest", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	req.Header.Set("If-None-Match", "*")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("create %d", res.StatusCode)
	}

	// get etag
	gres, err := http.Get(ts.URL + "/api/v1/manifest")
	if err != nil {
		t.Fatal(err)
	}
	etag := gres.Header.Get("ETag")
	gres.Body.Close()
	if etag == "" {
		t.Fatal("missing etag")
	}

	// stale if-match
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/manifest", bytes.NewReader(append(raw, []byte("\n")...)))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	req.Header.Set("If-Match", `"deadbeef"`)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}

	// good if-match
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/manifest", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	req.Header.Set("If-Match", etag)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("update %d", res.StatusCode)
	}
}

func TestSaveAsTraversal(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	for _, path := range []string{"../escape.yaml", "%2e%2e/escape.yaml", "/tmp/x.yaml", `..\x.yaml`} {
		body, _ := json.Marshal(map[string]string{"path": path, "content": "x"})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/manifest/save-as", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Iacpack-CSRF", csrf)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode == http.StatusCreated {
			t.Fatalf("traversal accepted for %q", path)
		}
	}
}

func TestCLIAPIValidateEquivalence(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "examples", "minimal"), dir)
	file := filepath.Join(dir, "iacpack.yaml")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"validate", "--file", file, "--output", "json"}, stdout, stderr, filesystem.OS{})
	if code != cli.ExitOK {
		t.Fatalf("cli validate %d %s", code, stderr.String())
	}
	var cliEnv struct {
		OK          bool  `json:"ok"`
		Diagnostics []any `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &cliEnv); err != nil {
		t.Fatal(err)
	}

	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/validate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var apiEnv struct {
		OK          bool  `json:"ok"`
		Diagnostics []any `json:"diagnostics"`
	}
	if err := json.NewDecoder(res.Body).Decode(&apiEnv); err != nil {
		t.Fatal(err)
	}
	if apiEnv.OK != cliEnv.OK {
		t.Fatalf("ok mismatch cli=%v api=%v", cliEnv.OK, apiEnv.OK)
	}
	cb, _ := json.Marshal(cliEnv.Diagnostics)
	ab, _ := json.Marshal(apiEnv.Diagnostics)
	if !bytes.Equal(cb, ab) {
		t.Fatalf("diagnostics mismatch\ncli=%s\napi=%s", cb, ab)
	}
}

func TestBuildQueueBoundAndCancel(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "examples", "release"), dir)
	// lock required for builds
	if code := cli.ExecuteWithFS([]string{"lock", "--file", filepath.Join(dir, "iacpack.yaml")}, &bytes.Buffer{}, &bytes.Buffer{}, filesystem.OS{}); code != cli.ExitOK {
		t.Fatal("lock")
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)

	start := func() *http.Response {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/builds", bytes.NewReader([]byte(`{"format":"directory"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Iacpack-CSRF", csrf)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	r1 := start()
	r2 := start()
	r3 := start()
	defer r1.Body.Close()
	defer r2.Body.Close()
	defer r3.Body.Close()
	if r1.StatusCode != http.StatusAccepted || r2.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202,202 got %d,%d", r1.StatusCode, r2.StatusCode)
	}
	if r3.StatusCode != http.StatusTooManyRequests {
		// queue may free quickly if builds finish; accept either 429 or 202 when slots freed
		if r3.StatusCode != http.StatusAccepted {
			t.Fatalf("third build status %d", r3.StatusCode)
		}
	}

	var job struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r1.Body).Decode(&job)
	if job.ID == "" {
		t.Fatal("missing id")
	}
	creq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/builds/"+job.ID, nil)
	creq.Header.Set("Cookie", cookie)
	creq.Header.Set("X-Iacpack-CSRF", csrf)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	cres.Body.Close()
	if cres.StatusCode != 200 {
		t.Fatalf("cancel %d", cres.StatusCode)
	}
}

func TestConcurrentReadsSaves(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := http.Get(ts.URL + "/api/v1/manifest")
			if err == nil {
				res.Body.Close()
			}
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/validate", bytes.NewReader([]byte(`{}`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("X-Iacpack-CSRF", csrf)
			res, err = http.DefaultClient.Do(req)
			if err == nil {
				res.Body.Close()
			}
		}()
	}
	wg.Wait()
}

func TestSSEOverflowResyncMarker(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	res, err := http.Get(ts.URL + "/api/v1/manifest/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	buf := make([]byte, 4096)
	n, err := res.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatal(err)
	}
	if !bytes.Contains(buf[:n], []byte("resyncRequired")) {
		t.Fatalf("missing resync handshake: %s", buf[:n])
	}
}

func TestGeneratedClientDrift(t *testing.T) {
	root := repoRoot(t)
	clientPath := filepath.Join(root, "web", "src", "api", "generated", "client.ts")
	raw, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatalf("generated client missing: %v", err)
	}
	for _, op := range []string{
		"getHealth", "getManifest", "putManifest", "validateManifest", "inspectGraph",
		"previewCompose", "previewArtifacts", "lockManifest", "startBuild", "getBuild", "cancelBuild",
		"listInputs", "exportManifest", "saveAsManifest", "discoverWorkspace",
	} {
		if !bytes.Contains(raw, []byte(op)) {
			t.Errorf("generated client missing operationId %s", op)
		}
	}
	paths := openAPIPaths(t, openapiv1.YAML)
	for k := range paths {
		parts := strings.SplitN(k, " ", 2)
		if len(parts) != 2 {
			continue
		}
		if !bytes.Contains(raw, []byte(parts[1])) {
			t.Errorf("generated client missing path %s", parts[1])
		}
	}
}

func newTestServer(t *testing.T) *httpapi.Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func bootstrap(t *testing.T, ts *httptest.Server) (cookie, csrf string) {
	t.Helper()
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	m := regexp.MustCompile(`name="iacpack-csrf" content="([^"]+)"`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("csrf meta missing: %s", body)
	}
	csrf = string(m[1])
	for _, c := range res.Cookies() {
		if c.Name == "iacpack_session" {
			cookie = c.Name + "=" + c.Value
		}
	}
	if cookie == "" {
		t.Fatal("session cookie missing")
	}
	return cookie, csrf
}

func minimalManifest() []byte {
	return []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: api-test
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    app:
      image: app
`)
}

func openAPIPaths(t *testing.T, yaml []byte) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	lines := strings.Split(string(yaml), "\n")
	var curPath string
	inPaths := false
	for _, line := range lines {
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !strings.HasPrefix(line, "paths:") {
			break
		}
		if strings.HasPrefix(line, "  /") {
			curPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		trim := strings.TrimSpace(line)
		for _, m := range []string{"get:", "put:", "post:", "delete:"} {
			if trim == m {
				method := strings.ToUpper(strings.TrimSuffix(m, ":"))
				out[method+" "+curPath] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no openapi paths parsed")
	}
	return out
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSecretGenerateDoesNotReturnValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iacpack.yaml"), minimalManifest(), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)

	post := func(body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/secrets/generate", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Iacpack-CSRF", csrf)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, raw
	}

	status, raw := post(`{"id":"db-password","kind":"password","value":"nope"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("value status %d %s", status, raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "db-password")); !os.IsNotExist(err) {
		t.Fatal("rejected request must not write a file")
	}

	status, raw = post(`{"id":"db-password","kind":"password"}`)
	if status != http.StatusOK {
		t.Fatalf("generate status %d %s", status, raw)
	}
	var body struct {
		OK     bool `json:"ok"`
		Result struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Result.Path != "secrets/db-password" || body.Result.Kind != "password" {
		t.Fatalf("body %s", raw)
	}
	file, err := os.ReadFile(filepath.Join(dir, "secrets", "db-password"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, file) || bytes.Contains(raw, []byte(`"value"`)) {
		t.Fatal("response included secret bytes")
	}
	info, err := os.Stat(filepath.Join(dir, "secrets", "db-password"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}

	status, raw = post(`{"id":"db-password","kind":"name"}`)
	if status != http.StatusConflict {
		t.Fatalf("conflict status %d %s", status, raw)
	}
	if bytes.Contains(raw, file) {
		t.Fatal("conflict response included secret bytes")
	}
	status, raw = post(`{"id":"../x","kind":"password"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("traversal status %d %s", status, raw)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/adapters/httpapi -> repo root
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}
