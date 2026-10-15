package apply

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/bundle"
)

func TestMaterializePayloadWritesFileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	// A previous compose up created the mount source as an empty directory.
	empty := filepath.Join(dir, "payload", "res-elasticsearch-elasticsearchyml")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	err := materializeBundlePayload(dir, map[string]bundle.Entry{
		"payload/res-elasticsearch-elasticsearchyml/elasticsearch.yml": {
			Path:    "payload/res-elasticsearch-elasticsearchyml/elasticsearch.yml",
			Mode:    "0644",
			Content: []byte("cluster.name: eliteew\n"),
		},
		"payload/res-eliteew-api-prod-api/ewa_platform/agentic/orchestration/__init__.py": {
			Path:    "payload/res-eliteew-api-prod-api/ewa_platform/agentic/orchestration/__init__.py",
			Mode:    "0644",
			Size:    0,
			Content: nil,
		},
		"payload/res-gotenberg-log-to-filesh/log-to-file.sh": {
			Path:    "payload/res-gotenberg-log-to-filesh/log-to-file.sh",
			Mode:    "0755",
			Content: []byte("#!/bin/sh\n"),
		},
		"compose.yaml": {Path: "compose.yaml", Content: []byte("x")},
	})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(empty, "elasticsearch.yml")
	st, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDir() {
		t.Fatal("config file was a directory")
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != "cluster.name: eliteew\n" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	initPy := filepath.Join(dir, "payload", "res-eliteew-api-prod-api", "ewa_platform", "agentic", "orchestration", "__init__.py")
	st, err = os.Lstat(initPy)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDir() || st.Size() != 0 {
		t.Fatalf("empty init.py mode dir=%v size=%d", st.IsDir(), st.Size())
	}
	script := filepath.Join(dir, "payload", "res-gotenberg-log-to-filesh", "log-to-file.sh")
	st, err = os.Lstat(script)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("script mode=%o", st.Mode().Perm())
	}
}

func TestMaterializeRejectsUnreadNonEmptyEntry(t *testing.T) {
	dir := t.TempDir()
	err := materializeBundlePayload(dir, map[string]bundle.Entry{
		"payload/res/file.txt": {Path: "payload/res/file.txt", Size: 4, Content: nil},
	})
	if err == nil {
		t.Fatal("expected read failure for a non-empty entry with no bytes")
	}
}

func TestRelabelPayloadWhenSELinuxPresent(t *testing.T) {
	if _, err := os.Stat("/sys/fs/selinux/enforce"); err != nil {
		t.Skip("selinux not present")
	}
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload", "res-redis-redis")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "redis.conf"), []byte("dir /data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := relabelContainerFiles(filepath.Join(dir, "payload")); err != nil {
		t.Fatal(err)
	}
}
