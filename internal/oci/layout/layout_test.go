package layout_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/oci/layout"
)

func TestPutVerifyAndTamper(t *testing.T) {
	root := t.TempDir()
	c, err := layout.New(root)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("hello-blob")
	sum := sha256.Sum256(data)
	d := "sha256:" + hex.EncodeToString(sum[:])
	if err := c.PutBlob(d, data); err != nil {
		t.Fatal(err)
	}
	if err := c.PutBlob(d, data); err != nil {
		t.Fatal("dedupe failed")
	}
	if err := c.Verify([]string{d}); err != nil {
		t.Fatal(err)
	}
	// swap content
	hexPart := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(root, "blobs", "sha256", hexPart), []byte("TAMPER"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify([]string{d}); err == nil {
		t.Fatal("expected tamper detection")
	}
}

func TestMissingDigest(t *testing.T) {
	root := t.TempDir()
	c, err := layout.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Verify([]string{"sha256:" + hex.EncodeToString(make([]byte, 32))}); err == nil {
		t.Fatal("expected missing")
	}
}
