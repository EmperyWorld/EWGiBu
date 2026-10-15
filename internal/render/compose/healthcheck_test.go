package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/render/compose"
)

func TestHealthcheckTestFromManifestShell(t *testing.T) {
	test := compose.HealthcheckTestFromManifest([]string{
		"/bin/sh", "-c", "curl -sf http://127.0.0.1/health >/dev/null",
	})
	if len(test) != 2 || test[0] != "CMD-SHELL" {
		t.Fatalf("test=%v", test)
	}
	if !strings.Contains(test[1], "curl") {
		t.Fatalf("script=%q", test[1])
	}
}

func TestHealthcheckTestFromManifestExec(t *testing.T) {
	test := compose.HealthcheckTestFromManifest([]string{"redis-cli", "ping"})
	if len(test) != 3 || test[0] != "CMD" || test[1] != "redis-cli" {
		t.Fatalf("test=%v", test)
	}
}

func TestMarshalHealthcheckCMDShell(t *testing.T) {
	f := compose.File{
		Name: "demo",
		Services: map[string]compose.Service{
			"app": {
				Image: "img",
				Healthcheck: &compose.Healthcheck{
					Test:     []string{"CMD-SHELL", "curl -f http://localhost/"},
					Interval: "30s",
					Timeout:  "10s",
					Retries:  3,
				},
			},
		},
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "CMD-SHELL") || !strings.Contains(s, "curl -f http://localhost/") {
		t.Fatalf("missing healthcheck:\n%s", s)
	}
}
