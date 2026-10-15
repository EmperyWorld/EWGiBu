// Package distribute builds a thin Linux distribution layout (bundle + runtime + install.sh).
package distribute

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Options configures distribution materialization.
type Options struct {
	BundlePath string
	RuntimeBin string // path to iacpack binary to embed
	OutputDir  string // new directory target
	Arch       string // amd64|arm64; default GOARCH
}

// Result is the distribute outcome.
type Result struct {
	OK    bool
	Error error
	Files []string
}

// InstallScript is the thin, ShellCheck-friendly installer (no orchestration).
const InstallScript = `#!/bin/sh
set -eu
DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BIN="$DIR/bin/iacpack"
SUMS="$DIR/SHA256SUMS"
BUNDLE="$DIR/bundle.iacbundle"
if [ ! -f "$BIN" ] || [ ! -f "$SUMS" ] || [ ! -f "$BUNDLE" ]; then
  echo "install.sh: missing distribution files" >&2
  exit 4
fi
# Verify checksums relative to distribution root.
( cd "$DIR" && sha256sum -c SHA256SUMS ) || {
  echo "install.sh: checksum verification failed" >&2
  exit 5
}
exec "$BIN" apply "$BUNDLE" "$@"
`

// Build creates the distribution directory (refuses existing target).
func Build(opts Options) Result {
	if opts.BundlePath == "" || opts.RuntimeBin == "" || opts.OutputDir == "" {
		return Result{Error: fmt.Errorf("bundle, runtime, and output required")}
	}
	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	if arch != "amd64" && arch != "arm64" {
		return Result{Error: fmt.Errorf("unsupported arch %q", arch)}
	}
	if _, err := os.Lstat(opts.OutputDir); err == nil {
		return Result{Error: fmt.Errorf("refusing to overwrite existing %s", opts.OutputDir)}
	} else if !os.IsNotExist(err) {
		return Result{Error: err}
	}
	staging := opts.OutputDir + ".iacpack-staging"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(filepath.Join(staging, "bin"), 0o755); err != nil {
		return Result{Error: err}
	}
	bundleData, err := os.ReadFile(opts.BundlePath)
	if err != nil {
		return Result{Error: err}
	}
	rtData, err := os.ReadFile(opts.RuntimeBin)
	if err != nil {
		return Result{Error: err}
	}
	files := map[string][]byte{
		"bundle.iacbundle": bundleData,
		"bin/iacpack":      rtData,
		"install.sh":       []byte(InstallScript),
		"README.md":        []byte("# iacpack distribution\n\nRun ./install.sh to verify checksums and apply the bundle.\n"),
	}
	var paths []string
	for p, data := range files {
		full := filepath.Join(staging, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return Result{Error: err}
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(p, "iacpack") || p == "install.sh" {
			mode = 0o755
		}
		if err := os.WriteFile(full, data, mode); err != nil {
			return Result{Error: err}
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var sums strings.Builder
	for _, p := range paths {
		if p == "SHA256SUMS" {
			continue
		}
		sum := sha256.Sum256(files[p])
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), p)
	}
	if err := os.WriteFile(filepath.Join(staging, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		return Result{Error: err}
	}
	paths = append(paths, "SHA256SUMS")
	sort.Strings(paths)
	if err := os.Rename(staging, opts.OutputDir); err != nil {
		_ = os.RemoveAll(staging)
		return Result{Error: err}
	}
	return Result{OK: true, Files: paths}
}
