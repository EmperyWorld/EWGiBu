// Package filesystem provides a narrow OS filesystem adapter for CLI commands.
package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// OS is a concrete filesystem bound to the local OS.
type OS struct{}

// ReadFile reads an entire file.
func (OS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Exists reports whether path exists.
func (OS) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// WriteFileAtomicReplace writes data via a same-directory temp file, fsyncs when
// supported, then atomically renames over path (creating or replacing).
// On failure the previous file (if any) is left unchanged.
func (OS) WriteFileAtomicReplace(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".iacpack-lock-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}
	cleanup = false
	// Best-effort directory fsync for durability on supporting platforms.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// WriteFileAtomic creates path exclusively via a same-directory temp file and
// hard-link publish. It never uses rename-to-existing, which could overwrite.
func (OS) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	exists, err := (OS{}).Exists(path)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("refusing to overwrite existing file %s", path)
	}
	tmp, err := os.CreateTemp(dir, ".iacpack-init-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Hard-link publish is atomic and fails if the destination already exists.
	if err := os.Link(tmpName, path); err == nil {
		cleanup = false
		_ = os.Remove(tmpName)
		return nil
	}

	if exists, err2 := (OS{}).Exists(path); err2 != nil {
		return err2
	} else if exists {
		return fmt.Errorf("refusing to overwrite existing file %s", path)
	}

	// Fallback when hard links are unavailable: exclusive create + copy.
	// Never os.Rename onto the destination — that can overwrite on Unix.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing file %s", path)
		}
		return fmt.Errorf("atomic exclusive create: %w", err)
	}
	removePartial := true
	defer func() {
		_ = f.Close()
		if removePartial {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write exclusive file: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		return fmt.Errorf("chmod exclusive file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close exclusive file: %w", err)
	}
	removePartial = false
	cleanup = false
	_ = os.Remove(tmpName)
	return nil
}
