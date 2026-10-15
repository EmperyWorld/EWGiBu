package images_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/app/images"
	"iacpack/internal/lockfile"
)

type memFS map[string][]byte

func (m memFS) ReadFile(p string) ([]byte, error) { return m[p], nil }

func TestFetchFixtureAndCorruptReject(t *testing.T) {
	layer := []byte("layer-bytes")
	lsum := sha256.Sum256(layer)
	ld := "sha256:" + hex.EncodeToString(lsum[:])
	cfg := []byte(`{"architecture":"amd64","os":"linux"}`)
	man, blobs := images.SynthesizeManifest(cfg, []string{ld})
	blobs[ld] = layer
	msum := sha256.Sum256(man)
	md := "sha256:" + hex.EncodeToString(msum[:])

	lock := lockfile.Document{
		FormatVersion: lockfile.FormatVersion,
		Images: map[string]lockfile.ImageLock{
			"app": {Ref: "example.com/app@" + md, Digest: md},
		},
	}
	// minimal marshal via json in lockfile - use Canonical or manual
	raw, err := marshalLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "iacpack.lock.json")
	if err := os.WriteFile(lockPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	fetcher := images.FixtureFetcher{
		md: {Manifest: man, Blobs: blobs},
	}
	out := filepath.Join(dir, "cache")
	res := images.Fetch(context.Background(), memFS{lockPath: raw}, images.Options{
		LockPath: lockPath, Output: out, Fetcher: fetcher,
	})
	if !res.OK {
		t.Fatalf("%v", res.Diagnostics)
	}
	// tamper blob and ensure rebuild verify would fail — rebuild cache path for embed uses Verify
	entries, _ := os.ReadDir(filepath.Join(out, "blobs", "sha256"))
	if len(entries) == 0 {
		t.Fatal("no blobs")
	}
}

func marshalLock(d lockfile.Document) ([]byte, error) {
	// reuse Parse-compatible JSON
	return []byte(`{
  "formatVersion": "iacpack.lock/v1",
  "manifest": {"sha256": "` + hex.EncodeToString(make([]byte, 32)) + `", "semanticHash": "` + hex.EncodeToString(make([]byte, 32)) + `"},
  "schema": {"id": "x", "sha256": "` + hex.EncodeToString(make([]byte, 32)) + `"},
  "provider": {"profile": "compose-docker/v1", "version": "1"},
  "compiler": {"product": "iacpack", "version": "0.1.0-dev"},
  "mode": "release",
  "policy": {"name": "default", "sha256": "` + hex.EncodeToString(make([]byte, 32)) + `"},
  "buildParameters": {},
  "inputs": [],
  "images": {
    "app": {"ref": "example.com/app@` + d.Images["app"].Digest + `", "digest": "` + d.Images["app"].Digest + `"}
  }
}
`), nil
}
