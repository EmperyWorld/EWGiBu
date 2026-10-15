// Package images implements network-enabled image cache fetch (explicit only).
package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"iacpack/internal/diagnostic"
	"iacpack/internal/lockfile"
	"iacpack/internal/oci"
	"iacpack/internal/oci/layout"
)

// Fetcher materializes one digest-pinned image into blob bytes (manifest + config + layers simplified).
// Production uses Docker CLI; tests use FixtureFetcher.
type Fetcher interface {
	Fetch(ctx context.Context, ref, digest string) (manifest []byte, blobs map[string][]byte, err error)
}

// FixtureFetcher serves preloaded digests (no network).
type FixtureFetcher map[string]struct {
	Manifest []byte
	Blobs    map[string][]byte
}

// Fetch implements Fetcher.
func (f FixtureFetcher) Fetch(ctx context.Context, ref, digest string) ([]byte, map[string][]byte, error) {
	_ = ctx
	_ = ref
	e, ok := f[digest]
	if !ok {
		return nil, nil, fmt.Errorf("fixture missing digest %s", digest)
	}
	return e.Manifest, e.Blobs, nil
}

// Options configures images fetch.
type Options struct {
	LockPath string
	Output   string
	Fetcher  Fetcher
}

// Result is the fetch outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Fetch builds a verified OCI layout cache from lockfile digests.
func Fetch(ctx context.Context, fs interface{ ReadFile(string) ([]byte, error) }, opts Options) Result {
	var res Result
	if opts.LockPath == "" || opts.Output == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"images fetch requires --lock and --output")}
		return finish(res)
	}
	if opts.Fetcher == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, "fetcher required")}
		return finish(res)
	}
	raw, err := fs.ReadFile(opts.LockPath)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	doc, err := lockfile.Parse(raw)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(lockfile.CodeLockParse, err.Error())}
		return finish(res)
	}
	if _, err := os.Lstat(opts.Output); err == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError,
			"refusing to overwrite existing image-cache "+opts.Output)}
		return finish(res)
	}
	staging := opts.Output + ".iacpack-staging"
	_ = os.RemoveAll(staging)
	cache, err := layout.New(staging)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}

	ids := make([]string, 0, len(doc.Images))
	for id := range doc.Images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var images []layout.ImageRef
	var digests []string
	for _, id := range ids {
		img := doc.Images[id]
		if img.Build {
			continue
		}
		ref := img.Ref
		digest := img.Digest
		if digest == "" {
			if d, ok := oci.DigestOf(ref); ok {
				digest = d
			}
		}
		if digest == "" {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockImageUnresolved,
				fmt.Sprintf("image %q has no digest", id)))
			continue
		}
		man, blobs, err := opts.Fetcher.Fetch(ctx, ref, digest)
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error("IMAGES.FETCH",
				fmt.Sprintf("fetch %s: %v", id, err)))
			continue
		}
		sum := sha256.Sum256(man)
		manDigest := "sha256:" + hex.EncodeToString(sum[:])
		if manDigest != digest {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error("IMAGES.FETCH",
				fmt.Sprintf("image %q manifest digest %s != lock %s", id, manDigest, digest)))
			continue
		}
		if err := cache.PutBlob(manDigest, man); err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error("IMAGES.FETCH", err.Error()))
			continue
		}
		for d, b := range blobs {
			if err := cache.PutBlob(d, b); err != nil {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("IMAGES.FETCH", err.Error()))
				continue
			}
			digests = append(digests, d)
		}
		digests = append(digests, manDigest)
		images = append(images, layout.ImageRef{
			ID: id, Ref: ref, Digest: manDigest,
			Manifest: layout.Descriptor{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    manDigest, Size: int64(len(man)),
			},
		})
	}
	if diagnostic.HasError(res.Diagnostics) {
		_ = os.RemoveAll(staging)
		return finish(res)
	}
	if err := cache.WriteIndex(images); err != nil {
		_ = os.RemoveAll(staging)
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	if err := cache.Verify(unique(digests)); err != nil {
		_ = os.RemoveAll(staging)
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("IMAGES.VERIFY", err.Error()))
		return finish(res)
	}
	if err := os.Rename(staging, opts.Output); err != nil {
		_ = os.RemoveAll(staging)
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	res.OK = true
	res.Model = map[string]any{"output": opts.Output, "images": len(images), "blobs": len(unique(digests))}
	res.Text = fmt.Sprintf("fetched %d images into %s\n", len(images), opts.Output)
	return finish(res)
}

func unique(in []string) []string {
	m := map[string]bool{}
	var out []string
	for _, s := range in {
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}

// SynthesizeManifest builds a tiny OCI manifest + config blob for tests/fixtures.
func SynthesizeManifest(config []byte, layerDigests []string) (manifest []byte, blobs map[string][]byte) {
	blobs = map[string][]byte{}
	cfgSum := sha256.Sum256(config)
	cfgDigest := "sha256:" + hex.EncodeToString(cfgSum[:])
	blobs[cfgDigest] = config
	type layer struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	}
	layers := make([]layer, 0, len(layerDigests))
	for _, d := range layerDigests {
		layers = append(layers, layer{
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    d, Size: int64(len(blobs[d])),
		})
	}
	man := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    cfgDigest,
			"size":      len(config),
		},
		"layers": layers,
	}
	raw, _ := json.Marshal(man)
	return raw, blobs
}
