// Package state defines versioned deployment state paths, locks, and DTOs (no journal writes in P07).
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FormatVersion is the state.json contract.
const FormatVersion = "iacpack.state/v1"

// Document is the durable per-instance state DTO (no secrets).
type Document struct {
	FormatVersion string          `json:"formatVersion"`
	Project       string          `json:"project"`
	Instance      string          `json:"instance"`
	ProviderSlug  string          `json:"providerSlug"`
	Active        *ActiveBundle   `json:"active,omitempty"`
	LastRun       *RunRef         `json:"lastRun,omitempty"`
	Owned         []OwnedResource `json:"owned,omitempty"`
	Recovery      []string        `json:"recovery,omitempty"`
}

// ActiveBundle identifies the last successful deployment.
type ActiveBundle struct {
	Digest       string `json:"digest"`
	SemanticHash string `json:"semanticHash"`
	Path         string `json:"path,omitempty"` // optional operator-local path; not authoritative
}

// RunRef identifies a journaled run.
type RunRef struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// OwnedResource is a managed provider resource ID with labels metadata (no credentials).
type OwnedResource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Migrator upgrades state documents between schema versions.
type Migrator interface {
	From() string
	To() string
	Migrate(raw []byte) ([]byte, error)
}

// Paths computes the state directory layout under an explicit state home.
type Paths struct {
	Root     string
	State    string
	Journal  string
	Lock     string
	Rendered string
	Secrets  string
	Logs     string
}

// ResolveHome returns XDG state home for iacpack (overrideable for tests).
func ResolveHome(xdgStateHome, home string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "iacpack")
	}
	if home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".local", "state", "iacpack")
}

// ForInstance returns paths for a project/instance under stateHome (iacpack root).
func ForInstance(stateHome, project, instance string) Paths {
	root := filepath.Join(stateHome, "projects", project, instance)
	return Paths{
		Root:     root,
		State:    filepath.Join(root, "state.json"),
		Journal:  filepath.Join(root, "journal.db"),
		Lock:     filepath.Join(root, "operation.lock"),
		Rendered: filepath.Join(root, "rendered"),
		Secrets:  filepath.Join(root, "secrets"),
		Logs:     filepath.Join(root, "logs"),
	}
}

// Lock holds an exclusive operation lock.
type Lock struct {
	path string
	f    *os.File
}

// AcquireLock creates the state root if needed and takes an exclusive lock file.
// plan/doctor must not call this for mutation-free flows.
func AcquireLock(p Paths) (*Lock, error) {
	if err := os.MkdirAll(p.Root, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p.Lock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{path: p.Lock, f: f}, nil
}

// Release unlocks and closes.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := flockUnlock(l.f)
	cerr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return cerr
}

// ParseDocument decodes state.json.
func ParseDocument(raw []byte) (Document, error) {
	var d Document
	if err := json.Unmarshal(raw, &d); err != nil {
		return Document{}, err
	}
	if d.FormatVersion != FormatVersion {
		return Document{}, fmt.Errorf("unsupported state formatVersion %q", d.FormatVersion)
	}
	return d, nil
}

// EnsurePermissions sets expected modes on an existing state tree (best-effort).
func EnsurePermissions(p Paths) error {
	for _, dir := range []string{p.Root, p.Rendered, p.Secrets, p.Logs} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			_ = os.Chmod(dir, 0o700)
		}
	}
	return nil
}

// JournalActionState enumerates journal action states (writes begin in P08).
type JournalActionState string

const (
	JournalPlanned   JournalActionState = "planned"
	JournalStarted   JournalActionState = "started"
	JournalSucceeded JournalActionState = "succeeded"
	JournalFailed    JournalActionState = "failed"
	JournalUnknown   JournalActionState = "unknown"
)

// Scratch stamp avoids unused import if time needed later for backups.
var _ = time.Time{}
var _ = strings.TrimSpace
