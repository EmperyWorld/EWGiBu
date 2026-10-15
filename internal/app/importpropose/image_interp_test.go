package importpropose

import "testing"

func TestImageRefParameterIDBareAndRequired(t *testing.T) {
	id, ok := imageRefParameterID("${STACK0_API_IMAGE}")
	if !ok || id != "stack0-api-image" {
		t.Fatalf("bare id=%q ok=%v", id, ok)
	}
	id, ok = imageRefParameterID("${STACK4_TIKA_BAKED_IMAGE:?Set STACK4_TIKA_BAKED_IMAGE}")
	if !ok || id != "stack4-tika-baked-image" {
		t.Fatalf("required id=%q ok=%v", id, ok)
	}
	if _, ok = imageRefParameterID("${TAG:-nginx:1}"); ok {
		t.Fatal("default form is not a parameter gap")
	}
}

func TestClassifyNestedImageRef(t *testing.T) {
	_, pid, ok := classifyImportImageRef("${STACK0_QUEUE_WORKER_IMAGE:-${STACK0_API_WORKER_IMAGE:-${STACK0_API_IMAGE}}}")
	if !ok || pid != "stack0-api-image" {
		t.Fatalf("nested no-literal pid=%q ok=%v", pid, ok)
	}
	_, pid, ok = classifyImportImageRef("${STACK0_CELERY_WORKER_IMAGE:-${STACK0_API_IMAGE}}")
	if !ok || pid != "stack0-api-image" {
		t.Fatalf("one nest pid=%q ok=%v", pid, ok)
	}
	lit, pid, ok := classifyImportImageRef("${STACK0_PINNED_IMAGE:-${STACK0_API_IMAGE:-docker.io/library/nginx:1.27}}")
	if !ok || pid != "" || lit != "docker.io/library/nginx:1.27" {
		t.Fatalf("nested literal lit=%q pid=%q ok=%v", lit, pid, ok)
	}
	if _, ok := imageRefParameterID("${TAG:-nginx:1}"); ok {
		t.Fatal("default form is not a bare parameter")
	}
}

func TestPullRefDigestPinned(t *testing.T) {
	if pullRefDigestPinned("docker.io/library/postgres:18.4") {
		t.Fatal("tag must not count as digest")
	}
	const pin = "docker.io/library/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !pullRefDigestPinned(pin) {
		t.Fatal("digest pin")
	}
	images := map[string]any{
		"postgres": map[string]any{"ref": "docker.io/library/postgres:18.4", "delivery": "pull"},
	}
	if !anyMutablePullRef(images) {
		t.Fatal("expected mutable")
	}
}
