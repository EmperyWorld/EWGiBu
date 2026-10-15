// Package envsanitize clears ambient Compose/Docker variables for provider calls.
package envsanitize

import (
	"os"
	"strings"
)

// ClearedPrefixes are ambient variable prefixes that must not affect provider argv.
var ClearedPrefixes = []string{
	"COMPOSE_",
	"DOCKER_HOST",
	"DOCKER_CONTEXT",
	"DOCKER_TLS",
	"DOCKER_CERT",
}

// CleanEnviron returns a copy of env with COMPOSE_*/DOCKER_HOST/DOCKER_CONTEXT
// and related ambient keys removed. Caller supplies base (usually os.Environ()).
func CleanEnviron(base []string) []string {
	out := make([]string, 0, len(base))
	for _, e := range base {
		key, _, _ := strings.Cut(e, "=")
		if shouldClear(key) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func shouldClear(key string) bool {
	if key == "DOCKER_HOST" || key == "DOCKER_CONTEXT" {
		return true
	}
	if strings.HasPrefix(key, "COMPOSE_") {
		return true
	}
	if strings.HasPrefix(key, "DOCKER_TLS") || strings.HasPrefix(key, "DOCKER_CERT") {
		return true
	}
	return false
}

// SelectDockerSocket returns the local Unix socket path to use.
// Empty dockerSocket means default /var/run/docker.sock.
// TCP/SSH schemes are rejected.
func SelectDockerSocket(dockerSocket string) (string, error) {
	s := strings.TrimSpace(dockerSocket)
	if s == "" {
		s = "/var/run/docker.sock"
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "tcp://") || strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ssh://") {
		return "", errSocket("TCP/SSH Docker endpoints are forbidden; use a local Unix socket")
	}
	s = strings.TrimPrefix(s, "unix://")
	if !strings.HasPrefix(s, "/") {
		return "", errSocket("docker socket must be an absolute local Unix path")
	}
	return s, nil
}

type socketError string

func errSocket(msg string) error    { return socketError(msg) }
func (e socketError) Error() string { return string(e) }

// AmbientHasContamination reports whether the current process environment still
// contains cleared keys (for tests asserting CleanEnviron was applied before exec).
func AmbientHasContamination() bool {
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if shouldClear(key) {
			return true
		}
	}
	return false
}
