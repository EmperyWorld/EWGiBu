package workspaceingest

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIngestFile(t *testing.T) {
	dir := t.TempDir()
	res, diags := Ingest(dir, Request{
		Mode:   "file",
		Target: "data/x.txt",
		File:   []byte("hello"),
	}, DefaultLimits())
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(res.Written) != 1 {
		t.Fatalf("written=%v", res.Written)
	}
	b, err := os.ReadFile(filepath.Join(dir, "data", "x.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatal(err)
	}
}

func TestIngestZipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../evil.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	_, diags := Ingest(dir, Request{Mode: "zip", Target: "data/pkg", File: buf.Bytes()}, DefaultLimits())
	if len(diags) == 0 {
		t.Fatal("expected rejection")
	}
}
