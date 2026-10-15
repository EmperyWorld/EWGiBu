package importpropose

import (
	"strings"
	"testing"
)

func TestVolumeModeSELinuxStaysWritable(t *testing.T) {
	if volumeModeReadOnly("z") || volumeModeReadOnly("Z") || volumeModeReadOnly("rw") || volumeModeReadOnly("rw,z") || volumeModeReadOnly("") {
		t.Fatal("selinux and rw modes are writable")
	}
	if !volumeModeReadOnly("ro") || !volumeModeReadOnly("ro,z") || !volumeModeReadOnly("z,ro") || !volumeModeReadOnly("Z,ro") {
		t.Fatal("ro must stay read-only beside a selinux flag")
	}
}

func TestSplitVolumeShort(t *testing.T) {
	src, tgt, mode, ok := splitVolumeShort("../logs/stack4/tika:/logs:z")
	if !ok || src != "../logs/stack4/tika" || tgt != "/logs" || mode != "z" {
		t.Fatalf("got %q %q %q ok=%v", src, tgt, mode, ok)
	}
	src, tgt, mode, ok = splitVolumeShort("${STACK2_LLAMA_MODELS_PATH:?STACK2_LLAMA_MODELS_PATH not set}:/models:ro,z")
	if !ok || tgt != "/models" || mode != "ro,z" || !strings.HasPrefix(src, "${STACK2_LLAMA_MODELS_PATH:") {
		t.Fatalf("llama bind got %q %q %q ok=%v", src, tgt, mode, ok)
	}
	src, tgt, mode, ok = splitVolumeShort("../scripts/log.sh:/usr/local/bin/log.sh:ro,z")
	if !ok || mode != "ro,z" || tgt != "/usr/local/bin/log.sh" {
		t.Fatalf("ro,z got %q %q %q ok=%v", src, tgt, mode, ok)
	}
}

func TestSplitOutsideInterpolationPort(t *testing.T) {
	parts := splitOutsideInterpolation(
		"${STACK1_CLIENT_BIND:-127.0.0.1}:${POSTGRES_HOST_PORT:?not set}:5432",
		':',
	)
	if len(parts) != 3 || parts[2] != "5432" {
		t.Fatalf("parts=%v", parts)
	}
}

func TestMapShortPortStack1BindDefault(t *testing.T) {
	params := map[string]any{}
	out, diags := mapShortPortString(
		"${STACK1_CLIENT_BIND:-127.0.0.1}:${CHROMA_HOST_PORT:?not set}:8000",
		"stack1.yml", "/services/chroma/ports/0", params, 1, 1,
	)
	if out == nil {
		t.Fatalf("nil port diags=%v", diags)
	}
	if out["bind"] != "127.0.0.1" || out["container"] != 8000 {
		t.Fatalf("out=%v", out)
	}
}

func TestMountBindFileSourceIsFileNotDirectory(t *testing.T) {
	resources := map[string]any{}
	m, diags := mountBindFromSource("docker/stack/stack1.yml", "elasticsearch", "stack1.yml", "/volumes/0",
		"docker/data/stack1/elasticsearch_config/elasticsearch.yml",
		"/usr/share/elasticsearch/config/elasticsearch.yml", true, resources, nil, false)
	if len(diags) != 0 || m == nil {
		t.Fatalf("mount=%v diags=%v", m, diags)
	}
	if m["source"] != "elasticsearch.yml" || m["target"] != "/usr/share/elasticsearch/config/elasticsearch.yml" {
		t.Fatalf("mount=%v", m)
	}
	if _, bundledDir := m["host"]; bundledDir {
		t.Fatal("file bind must stay a resource file, not an external host path")
	}
}

func TestMountBindSourceInterpDefault(t *testing.T) {
	resources := map[string]any{}
	m, diags := mountBindFromSource("docker/stack/stack.yml", "app", "f.yml", "/volumes/0",
		"${MODELS:-data/models}", "/models", true, resources, nil, false)
	if m == nil || len(diags) != 0 {
		t.Fatalf("mount=%v diags=%v", m, diags)
	}
	if len(resources) != 1 {
		t.Fatalf("resources=%v", resources)
	}
}

func TestMapShortPortThreePart(t *testing.T) {
	params := map[string]any{}
	out, diags := mapShortPortString(
		"127.0.0.1:${STACK4_TIKA_HOST_PORT:?not set}:9998",
		"f.yml", "/services/tika/ports/0", params, 1, 1,
	)
	if out == nil {
		t.Fatalf("nil port diags=%v", diags)
	}
	if out["bind"] != "127.0.0.1" || out["container"] != 9998 {
		t.Fatalf("out=%v", out)
	}
	host, ok := out["host"].(map[string]any)
	if !ok || host["parameter"] == nil {
		t.Fatalf("host=%v params=%v", out["host"], params)
	}
}
