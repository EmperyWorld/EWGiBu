package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/httpapi"
	"iacpack/internal/policy"
)

func TestProjectsStayInsideStudio(t *testing.T) {
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

	post := func(path string, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader([]byte(body)))
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
		return res
	}

	res := post("/api/v1/projects", `{"name":"../etc"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("escape status %d", res.StatusCode)
	}

	res = post("/api/v1/projects", `{"name":"shop-a"}`)
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %s", res.StatusCode, raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "projects", "shop-a", "iacpack.yaml")); err != nil {
		t.Fatal(err)
	}

	info := getJSON(t, ts, "/api/v1/workspace")
	ws, _ := info["workspace"].(string)
	if !strings.HasSuffix(filepath.ToSlash(ws), "/projects/shop-a") {
		t.Fatalf("workspace=%s", ws)
	}

	res = post("/api/v1/projects/select", `{"id":"workspace"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("select root %d", res.StatusCode)
	}
	info = getJSON(t, ts, "/api/v1/workspace")
	ws, _ = info["workspace"].(string)
	if filepath.Clean(ws) != filepath.Clean(dir) {
		t.Fatalf("root workspace=%s", ws)
	}
}

func getJSON(t *testing.T, ts *httptest.Server, path string) map[string]any {
	t.Helper()
	res, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}
