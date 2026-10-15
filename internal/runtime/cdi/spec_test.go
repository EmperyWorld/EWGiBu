package cdi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeclaresNVIDIAAll(t *testing.T) {
	dir := t.TempDir()
	if Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("empty dir")
	}
	body := []byte("kind: nvidia.com/gpu\ndevices:\n  - name: \"0\"\n")
	if err := os.WriteFile(filepath.Join(dir, "nvidia.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("index 0 is not all")
	}
	body = []byte("cdiVersion: \"0.5.0\"\nkind: nvidia.com/gpu\ndevices:\n  - name: all\n    containerEdits:\n      env:\n        - NVIDIA_VISIBLE_DEVICES=void\n")
	if err := os.WriteFile(filepath.Join(dir, "nvidia.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if !Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("yaml name all")
	}
}

func TestDeclaresJSONAndOtherKind(t *testing.T) {
	dir := t.TempDir()
	other := []byte("{\"kind\":\"vendor.example/device\",\"devices\":[{\"name\":\"all\"}]}\n")
	if err := os.WriteFile(filepath.Join(dir, "other.json"), other, 0o644); err != nil {
		t.Fatal(err)
	}
	if Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("other kind")
	}
	js := []byte("{\"cdiVersion\":\"0.6.0\",\"kind\":\"nvidia.com/gpu\",\"devices\":[{\"name\":\"all\"}]}\n")
	if err := os.WriteFile(filepath.Join(dir, "nvidia.json"), js, 0o644); err != nil {
		t.Fatal(err)
	}
	if !Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("json name all")
	}
}

func TestDeclaresIgnoresComment(t *testing.T) {
	dir := t.TempDir()
	body := []byte("kind: nvidia.com/gpu\n# devices:\n#   - name: all\ndevices:\n  - name: \"0\"\n")
	if err := os.WriteFile(filepath.Join(dir, "nvidia.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if Declares([]string{dir}, "nvidia.com/gpu", "all") {
		t.Fatal("comment must not declare all")
	}
}

func TestCDISpecDirsFromConf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "containers.conf")
	text := "[engine]\n#cdi_spec_dirs = [\"/nope\"]\ncdi_spec_dirs = [\n  \"/etc/cdi\",\n  \"/var/run/cdi\",\n  \"/tmp/cdi\",\n]\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := cdiSpecDirsFromConf(path)
	if !ok {
		t.Fatal("expected key")
	}
	want := []string{"/etc/cdi", "/var/run/cdi", "/tmp/cdi"}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v", got)
		}
	}
	commented := filepath.Join(dir, "commented.conf")
	if err := os.WriteFile(commented, []byte("#cdi_spec_dirs = [\"/nope\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := cdiSpecDirsFromConf(commented); ok {
		t.Fatal("commented key must not set dirs")
	}
}
