package httpapi_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iacpack/internal/adapters/httpapi"
	"iacpack/internal/policy"
)

func readSSEUntil(t *testing.T, body io.Reader, needle string, timeout time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch := make(chan string, 1)
	go func() {
		var b strings.Builder
		r := bufio.NewReader(body)
		for {
			line, err := r.ReadString('\n')
			b.WriteString(line)
			if strings.Contains(b.String(), needle) {
				ch <- b.String()
				return
			}
			if err != nil {
				ch <- b.String()
				return
			}
		}
	}()
	select {
	case s := <-ch:
		if !strings.Contains(s, needle) {
			t.Fatalf("timeout/missing %q in %q", needle, s)
		}
		return s
	case <-ctx.Done():
		t.Fatalf("timeout waiting for %q", needle)
		return ""
	}
}

func TestWatcherEmitsChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iacpack.yaml")
	initial := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: w
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
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0", Policy: policy.MustDefault()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/api/v1/manifest/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	_ = readSSEUntil(t, res.Body, "resyncRequired", 2*time.Second)

	ext := append(append([]byte{}, initial...), []byte("\n# external\n")...)
	if err := os.WriteFile(path, ext, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = readSSEUntil(t, res.Body, `"kind":"changed"`, 4*time.Second)
}

func TestWatcherDeletedAndInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iacpack.yaml")
	good := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: x
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
	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	res, err := http.Get(ts.URL + "/api/v1/manifest/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	_ = readSSEUntil(t, res.Body, "resyncRequired", 2*time.Second)

	if err := os.WriteFile(path, []byte("not: a project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = readSSEUntil(t, res.Body, `"kind":"invalid"`, 4*time.Second)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_ = readSSEUntil(t, res.Body, `"kind":"deleted"`, 4*time.Second)
}

func TestSelfSaveDoesNotLoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iacpack.yaml")
	body := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: s
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
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Config{Workspace: dir, Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	cookie, csrf := bootstrap(t, ts)

	res, err := http.Get(ts.URL + "/api/v1/manifest/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	_ = readSSEUntil(t, res.Body, "resyncRequired", 2*time.Second)

	// GET etag then PUT (self save)
	gres, err := http.Get(ts.URL + "/api/v1/manifest")
	if err != nil {
		t.Fatal(err)
	}
	etag := gres.Header.Get("ETag")
	gres.Body.Close()

	newBody := append(body, []byte("\n")...)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/manifest", strings.NewReader(string(newBody)))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Iacpack-CSRF", csrf)
	req.Header.Set("If-Match", etag)
	pres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()
	if pres.StatusCode != 200 {
		t.Fatalf("put %d", pres.StatusCode)
	}

	// Should not see changed within debounce window+
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	ch := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(res.Body)
		ch <- string(b)
	}()
	select {
	case s := <-ch:
		if strings.Contains(s, `"kind":"changed"`) {
			t.Fatalf("self-save leaked changed event: %s", s)
		}
	case <-ctx.Done():
		// no event — good (read still blocked)
	}
}
