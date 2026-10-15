package apply

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"iacpack/internal/bundle"
)

// materializeBundlePayload writes bundle payload files into the rendered tree.
// A payload path that is a file is written as a file. An empty directory left
// in its place by an earlier compose up is removed first.
func materializeBundlePayload(rendered string, entries map[string]bundle.Entry) error {
	var wrote bool
	for _, e := range entries {
		if !strings.HasPrefix(e.Path, "payload/") || e.Path == "payload/" {
			continue
		}
		body := e.Content
		if body == nil {
			if e.Size != 0 {
				return fmt.Errorf("bundle entry %s could not be read", e.Path)
			}
			body = []byte{}
		}
		full := filepath.Join(rendered, filepath.FromSlash(e.Path))
		if st, err := os.Lstat(full); err == nil && st.IsDir() {
			if err := os.RemoveAll(full); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if e.Mode == "0755" {
			perm = 0o755
		}
		if err := os.WriteFile(full, body, perm); err != nil {
			return err
		}
		// WriteFile applies the process umask. Chmod keeps the execute bit.
		if err := os.Chmod(full, perm); err != nil {
			return err
		}
		wrote = true
	}
	if !wrote {
		return nil
	}
	return nil
}

// relabelMarker is embedded in the binary so a stale iacpack can be recognized.
const relabelMarker = "iacpack-relabel-podman-unshare"

// relabelContainerFiles sets container_file_t on a rendered tree so container_t
// can read it under SELinux enforcing. No-op when SELinux is absent.
// podman unshare is required after a writable mount was chowned to the image
// uid: that directory is often mode 0700 and the host user cannot traverse it.
// A directory the container itself created (Postgres pgdata) is relabeled again
// by the bind's :z option if this walk cannot read it, so that case does not
// stop apply.
func relabelContainerFiles(dir string) error {
	enf, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return nil
	}
	st, err := os.Lstat(dir)
	if err != nil || st.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !st.IsDir() && !st.Mode().IsRegular() {
		return nil
	}
	_ = relabelMarker
	if _, lookErr := exec.LookPath("podman"); lookErr == nil {
		cmd := exec.Command("podman", "unshare", "chcon", "-R", "-t", "container_file_t", dir)
		out, err := cmd.CombinedOutput()
		if err == nil || strings.TrimSpace(string(enf)) != "1" || relabelDenied(string(out)) {
			return nil
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", relabelMarker, msg)
	}
	cmd := exec.Command("chcon", "-R", "-t", "container_file_t", dir)
	out, err := cmd.CombinedOutput()
	if err != nil && strings.TrimSpace(string(enf)) == "1" && !relabelDenied(string(out)) {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", relabelMarker, msg)
	}
	return nil
}

func relabelDenied(msg string) bool {
	low := strings.ToLower(msg)
	return strings.Contains(low, "permission denied") ||
		strings.Contains(msg, "Отказано в доступе") ||
		strings.Contains(low, "operation not permitted")
}
