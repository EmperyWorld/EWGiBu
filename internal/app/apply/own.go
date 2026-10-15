package apply

import (
	"path/filepath"
	"strings"

	"iacpack/internal/domain/ir"
)

// writableTargets are host directories and compose volume names that a
// non-root service must be able to write. Read-only mounts are omitted.
func writableTargets(rendered string, svc ir.Service, deploy map[string]string) (binds, volumes []string) {
	for _, m := range svc.Mounts {
		if m.ReadOnly {
			continue
		}
		switch {
		case m.Volume != "":
			volumes = append(volumes, m.Volume)
		case m.Resource != "":
			p := filepath.Join(rendered, "payload", m.Resource)
			if m.Source != "" {
				p = filepath.Join(p, filepath.FromSlash(m.Source))
			}
			binds = append(binds, p)
		case m.Build != "":
			binds = append(binds, filepath.Join(rendered, "payload", "build-"+m.Build))
		case m.HostLiteral != "" || m.HostParameter != "":
			src := m.HostLiteral
			if m.HostParameter != "" {
				src = deploy[m.HostParameter]
			}
			if src != "" {
				binds = append(binds, src)
			}
		}
	}
	return binds, volumes
}

// hostBindSources are host paths mounted into the container, including
// read-only ones such as a models directory. SELinux must label them
// container_file_t or the process gets Permission denied on a file it can
// otherwise read.
func hostBindSources(svc ir.Service, deploy map[string]string) []string {
	var out []string
	for _, m := range svc.Mounts {
		if m.HostLiteral == "" && m.HostParameter == "" {
			continue
		}
		src := m.HostLiteral
		if m.HostParameter != "" {
			src = deploy[m.HostParameter]
		}
		if safeOwnPath(src) {
			out = append(out, src)
		}
	}
	return out
}

func relabelHostBinds(svc ir.Service, deploy map[string]string) error {
	for _, src := range hostBindSources(svc, deploy) {
		if err := relabelContainerFiles(src); err != nil {
			return err
		}
	}
	return nil
}

// parseNumericUser returns uid and gid for podman unshare chown.
// Root and non-numeric names are skipped: inside rootless Podman the host
// user already appears as uid 0.
func parseNumericUser(user string) (uid, gid string, ok bool) {
	user = strings.TrimSpace(user)
	if user == "" || user == "0" || user == "0:0" {
		return "", "", false
	}
	if strings.Contains(user, ":") {
		parts := strings.Split(user, ":")
		if len(parts) != 2 || !allDigits(parts[0]) || !allDigits(parts[1]) || parts[0] == "0" {
			return "", "", false
		}
		return parts[0], parts[1], true
	}
	if !allDigits(user) {
		return "", "", false
	}
	return user, user, true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func safeOwnPath(p string) bool {
	c := filepath.Clean(p)
	if !filepath.IsAbs(c) || c == string(filepath.Separator) {
		return false
	}
	return true
}
