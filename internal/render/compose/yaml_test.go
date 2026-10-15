package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/render/compose"
)

func TestMarshalDeterministicOrder(t *testing.T) {
	f := compose.File{
		Name: "demo",
		Services: map[string]compose.Service{
			"b": {Image: "img-b"},
			"a": {Image: "img-a", Environment: map[string]string{"Z": "1", "A": "2"}},
		},
		Networks: map[string]compose.Network{"n2": {}, "n1": {}},
	}
	a, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("unstable")
	}
	s := string(a)
	if !strings.Contains(s, "services:\n  a:\n") {
		t.Fatalf("services not sorted:\n%s", s)
	}
	idxA := strings.Index(s, "\n  a:\n")
	idxB := strings.Index(s, "\n  b:\n")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("order:\n%s", s)
	}
	if strings.Contains(s, "\r") {
		t.Fatal("CR present")
	}
}

func TestMarshalTaskProfiles(t *testing.T) {
	f := compose.File{
		Name: "demo",
		Services: map[string]compose.Service{
			"app":  {Image: "img-app", Restart: "unless-stopped"},
			"seed": {Image: "img-app", Command: []string{"/seed"}, Restart: "no", Profiles: []string{"iacpack-task"}},
		},
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "profiles:\n") || !strings.Contains(s, "- iacpack-task") {
		t.Fatalf("missing profiles:\n%s", s)
	}
	if !strings.Contains(s, "restart: \"no\"") && !strings.Contains(s, "restart: no") {
		t.Fatalf("missing restart no:\n%s", s)
	}
}

func TestMarshalDeployGPU(t *testing.T) {
	f := compose.File{
		Name: "demo",
		Services: map[string]compose.Service{
			"llama": {Image: "img", Deploy: &compose.Deploy{GPU: 1}},
		},
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "driver: nvidia") || !strings.Contains(s, "capabilities: [gpu]") || !strings.Contains(s, "count: 1") {
		t.Fatalf("gpu deploy:\n%s", s)
	}
}

func TestMarshalQuotesYAML11BoolCommandArgs(t *testing.T) {
	f := compose.File{
		Name: "demo",
		Services: map[string]compose.Service{
			"redis": {
				Image:   "redis",
				Command: []string{"redis-server", "--appendonly", "yes", "--protected-mode", "no"},
				Volumes: []compose.VolumeMount{
					{Type: "bind", Source: "./data", Target: "/data", ReadOnly: false, Relabel: "z"},
					{Type: "bind", Source: "./conf", Target: "/etc/redis.conf", ReadOnly: true, Relabel: "z"},
				},
			},
			"llama": {
				Image:   "llama",
				Command: []string{"--flash-attn", "on", "--reasoning", "off"},
			},
		},
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`- "yes"`, `- "no"`, `- "on"`, `- "off"`, `read_only: false`, `read_only: true`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n      - yes\n") || strings.Contains(s, "\n      - no\n") {
		t.Fatalf("unquoted YAML 1.1 bool slipped through:\n%s", s)
	}
}
