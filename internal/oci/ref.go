// Package oci parses OCI references and resolves digests via a narrow adapter.
package oci

import (
	"fmt"
	"strings"
)

// DigestPinned reports whether ref already contains an @sha256:<64-hex> digest.
func DigestPinned(ref string) bool {
	_, ok := DigestOf(ref)
	return ok
}

// DigestOf returns the sha256 digest suffix when present.
func DigestOf(ref string) (string, bool) {
	i := strings.LastIndex(ref, "@")
	if i < 0 {
		return "", false
	}
	d := ref[i+1:]
	if !strings.HasPrefix(d, "sha256:") {
		return "", false
	}
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) != 64 {
		return "", false
	}
	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return d, true
}

// WithDigest returns name@digest for a name (tag stripped) and digest.
func WithDigest(ref, digest string) string {
	base := ref
	if i := strings.Index(ref, "@"); i >= 0 {
		base = ref[:i]
	} else if i := strings.LastIndex(ref, ":"); i >= 0 {
		if slash := strings.LastIndex(ref, "/"); slash >= 0 && i > slash {
			base = ref[:i]
		} else if !strings.Contains(ref, "/") {
			base = ref[:i]
		}
	}
	return base + "@" + digest
}

// Resolver resolves a mutable tag reference to a digest-pinned reference.
// Implementations may use the network; callers must only invoke when explicitly allowed.
type Resolver interface {
	Resolve(ref string) (pinned string, digest string, err error)
}

// DeniedResolver always fails closed (default when network is not allowed).
type DeniedResolver struct{}

func (DeniedResolver) Resolve(ref string) (string, string, error) {
	return "", "", fmt.Errorf("network image resolution is disabled (pass --update-images)")
}

// MapResolver is a fake registry for tests (no network).
type MapResolver map[string]string // tag ref -> digest (sha256:...)

func (m MapResolver) Resolve(ref string) (string, string, error) {
	d, ok := m[ref]
	if !ok {
		return "", "", fmt.Errorf("fake registry: unknown ref %q", ref)
	}
	if !strings.HasPrefix(d, "sha256:") {
		return "", "", fmt.Errorf("fake registry: digest must use sha256 hex form")
	}
	return WithDigest(ref, d), d, nil
}
