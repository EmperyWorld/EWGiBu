// Package layout implements a minimal deterministic OCI image-layout cache (stdlib-only).
package layout

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LayoutVersion is written to oci-layout.
const LayoutVersion = "1.0.0"

// Descriptor is an OCI descriptor.
type Descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// Index is a minimal OCI image index.
type Index struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []Descriptor `json:"manifests"`
}

// ImageRef ties a lock image id to its digest-pinned ref and manifest descriptor.
type ImageRef struct {
	ID       string
	Ref      string
	Digest   string // sha256:...
	Manifest Descriptor
}

// Cache is an on-disk OCI layout rooted at Root.
type Cache struct {
	Root string
}

// New creates the layout directories and oci-layout file.
func New(root string) (*Cache, error) {
	c := &Cache{Root: root}
	if err := os.MkdirAll(c.blobsDir(), 0o755); err != nil {
		return nil, err
	}
	meta := map[string]string{"imageLayoutVersion": LayoutVersion}
	raw, _ := json.Marshal(meta)
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(root, "oci-layout"), raw, 0o644); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Cache) blobsDir() string { return filepath.Join(c.Root, "blobs", "sha256") }

// PutBlob writes content-addressed bytes; digest must match sha256 of data.
func (c *Cache) PutBlob(digest string, data []byte) error {
	hexPart, err := hexOf(digest)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hexPart {
		return fmt.Errorf("blob digest mismatch for %s", digest)
	}
	path := filepath.Join(c.blobsDir(), hexPart)
	if _, err := os.Lstat(path); err == nil {
		return nil // dedupe
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// GetBlob reads a blob by digest.
func (c *Cache) GetBlob(digest string) ([]byte, error) {
	hexPart, err := hexOf(digest)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(c.blobsDir(), hexPart))
}

// WriteIndex writes sorted index.json and images.json catalog.
func (c *Cache) WriteIndex(images []ImageRef) error {
	sort.SliceStable(images, func(i, j int) bool { return images[i].ID < images[j].ID })
	idx := Index{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
	}
	catalog := make([]map[string]any, 0, len(images))
	for _, img := range images {
		idx.Manifests = append(idx.Manifests, img.Manifest)
		catalog = append(catalog, map[string]any{
			"id": img.ID, "ref": img.Ref, "digest": img.Digest,
			"manifest": img.Manifest,
		})
	}
	if err := writeCanonicalJSON(filepath.Join(c.Root, "index.json"), idx); err != nil {
		return err
	}
	return writeCanonicalJSON(filepath.Join(c.Root, "images.json"), catalog)
}

// Verify checks oci-layout, every blob digest, and that each image digest has a blob.
func (c *Cache) Verify(expectedDigests []string) error {
	raw, err := os.ReadFile(filepath.Join(c.Root, "oci-layout"))
	if err != nil {
		return fmt.Errorf("oci-layout: %w", err)
	}
	if !strings.Contains(string(raw), LayoutVersion) {
		return fmt.Errorf("unsupported oci-layout version")
	}
	seen := map[string]bool{}
	err = filepath.Walk(c.blobsDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if strings.HasSuffix(name, ".tmp") {
			return fmt.Errorf("staging blob left behind: %s", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != name {
			return fmt.Errorf("corrupt blob %s (hash %s)", name, got)
		}
		seen["sha256:"+got] = true
		return nil
	})
	if err != nil {
		return err
	}
	for _, d := range expectedDigests {
		if !seen[d] {
			return fmt.Errorf("missing blob for %s", d)
		}
	}
	return nil
}

// CopyInto copies the entire layout into dest (new directory).
func (c *Cache) CopyInto(dest string) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("destination exists: %s", dest)
	}
	return copyTree(c.Root, dest)
}

func hexOf(digest string) (string, error) {
	if !strings.HasPrefix(digest, "sha256:") {
		return "", fmt.Errorf("digest must be sha256:hex")
	}
	h := strings.TrimPrefix(digest, "sha256:")
	if len(h) != 64 {
		return "", fmt.Errorf("invalid digest length")
	}
	return h, nil
}

func writeCanonicalJSON(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
