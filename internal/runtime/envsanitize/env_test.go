package envsanitize_test

import (
	"strings"
	"testing"

	"iacpack/internal/runtime/envsanitize"
)

func TestCleanEnviron(t *testing.T) {
	in := []string{"PATH=/bin", "COMPOSE_PROJECT_NAME=x", "DOCKER_HOST=tcp://1:2", "DOCKER_CONTEXT=foo", "HOME=/tmp"}
	out := envsanitize.CleanEnviron(in)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "COMPOSE_") || strings.Contains(joined, "DOCKER_HOST") || strings.Contains(joined, "DOCKER_CONTEXT") {
		t.Fatalf("contamination remains: %v", out)
	}
	if !strings.Contains(joined, "PATH=") {
		t.Fatal("lost PATH")
	}
}

func TestSelectDockerSocket(t *testing.T) {
	if _, err := envsanitize.SelectDockerSocket("tcp://1.2.3.4:2375"); err == nil {
		t.Fatal("tcp must fail")
	}
	s, err := envsanitize.SelectDockerSocket("")
	if err != nil || s != "/var/run/docker.sock" {
		t.Fatalf("got %q %v", s, err)
	}
}
