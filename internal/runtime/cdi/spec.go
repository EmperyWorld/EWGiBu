// Package cdi reads host Container Device Interface specs.
// It does not create specs or device nodes.
package cdi

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxSpecBytes = 1 << 20

// SpecDirs returns CDI spec directories from containers.conf, or the
// defaults /etc/cdi and /var/run/cdi when no file sets cdi_spec_dirs.
func SpecDirs() []string {
	dirs := []string{"/etc/cdi", "/var/run/cdi"}
	for _, path := range containersConfPaths() {
		if found, ok := cdiSpecDirsFromConf(path); ok {
			dirs = found
		}
	}
	return dirs
}

// Declares reports whether any spec in dirs has kind and device name.
// name is the short CDI device name (for example all), not vendor/class=name.
func Declares(dirs []string, kind, name string) bool {
	if kind == "" || name == "" {
		return false
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			base := ent.Name()
			if base != filepath.Base(base) {
				continue
			}
			ext := strings.ToLower(filepath.Ext(base))
			if ext != ".json" && ext != ".yaml" && ext != ".yml" {
				continue
			}
			body, err := readLimited(filepath.Join(dir, base), maxSpecBytes)
			if err != nil || len(body) == 0 {
				continue
			}
			if specDeclares(body, ext, kind, name) {
				return true
			}
		}
	}
	return false
}

func containersConfPaths() []string {
	paths := []string{
		"/usr/share/containers/containers.conf",
		"/etc/containers/containers.conf",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "containers", "containers.conf"))
	}
	if extra := strings.TrimSpace(os.Getenv("CONTAINERS_CONF")); extra != "" {
		paths = append(paths, extra)
	}
	return paths
}

func cdiSpecDirsFromConf(path string) ([]string, bool) {
	b, err := readLimited(path, maxSpecBytes)
	if err != nil {
		return nil, false
	}
	lines := strings.Split(string(b), "\n")
	for i := 0; i < len(lines); i++ {
		line := stripTOMLComment(lines[i])
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "cdi_spec_dirs") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trim, "cdi_spec_dirs"))
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rest = strings.TrimSpace(rest[1:])
		buf := rest
		for strings.Count(buf, "[") > strings.Count(buf, "]") && i+1 < len(lines) {
			i++
			buf += "\n" + stripTOMLComment(lines[i])
		}
		return parseTOMLStringArray(buf), true
	}
	return nil, false
}

func specDeclares(data []byte, ext, kind, name string) bool {
	if ext == ".json" || (len(data) > 0 && data[0] == '{') {
		if jsonDeclares(data, kind, name) {
			return true
		}
	}
	return yamlDeclares(data, kind, name)
}

type cdiSpec struct {
	Kind    string `json:"kind"`
	Devices []struct {
		Name string `json:"name"`
	} `json:"devices"`
}

func jsonDeclares(data []byte, kind, name string) bool {
	var spec cdiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return false
	}
	return kindHasDevice(spec.Kind, deviceNames(spec), kind, name)
}

func deviceNames(spec cdiSpec) []string {
	out := make([]string, 0, len(spec.Devices))
	for _, d := range spec.Devices {
		if d.Name != "" {
			out = append(out, d.Name)
		}
	}
	return out
}

func kindHasDevice(specKind string, names []string, kind, name string) bool {
	if specKind != kind {
		return false
	}
	full := kind + "=" + name
	for _, n := range names {
		if n == name || n == full {
			return true
		}
	}
	return false
}

func yamlDeclares(data []byte, kind, name string) bool {
	var specKind string
	var names []string
	devicesIndent := -1
	inDevices := false
	for _, line := range strings.Split(string(data), "\n") {
		raw := stripYAMLComment(line)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := yamlIndent(raw)
		trim := strings.TrimSpace(raw)
		if !inDevices && indent == 0 {
			if v, ok := yamlField(trim, "kind"); ok {
				specKind = v
			}
		}
		if strings.HasPrefix(trim, "devices:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trim, "devices:"))
			devicesIndent = indent
			if rest == "" || rest == "|" || rest == ">" {
				inDevices = true
				continue
			}
			inDevices = false
			names = append(names, flowDeviceNames(rest)...)
			continue
		}
		if inDevices {
			if indent <= devicesIndent && !strings.HasPrefix(trim, "-") {
				inDevices = false
			} else if v, ok := yamlField(trim, "name"); ok {
				names = append(names, v)
			}
		}
	}
	return kindHasDevice(specKind, names, kind, name)
}

func flowDeviceNames(s string) []string {
	var out []string
	rest := s
	for {
		i := strings.Index(rest, "name")
		if i < 0 {
			return out
		}
		frag := rest[i:]
		if v, ok := yamlField(strings.TrimSpace(frag), "name"); ok {
			out = append(out, v)
		} else if strings.HasPrefix(strings.TrimSpace(frag), "\"name\"") {
			val := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(frag), "\"name\""))
			val = strings.TrimSpace(strings.TrimPrefix(val, ":"))
			if v, ok := takeYAMLScalar(val); ok {
				out = append(out, v)
			}
		}
		if len(rest) < i+4 {
			return out
		}
		rest = rest[i+4:]
	}
}

func yamlField(trim, key string) (string, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(trim), "-"))
	p := key + ":"
	if !strings.HasPrefix(s, p) {
		return "", false
	}
	return takeYAMLScalar(strings.TrimSpace(s[len(p):]))
}

func takeYAMLScalar(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if s[0] == '"' || s[0] == '\'' {
		q := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == q {
				return s[1:i], true
			}
		}
		return "", false
	}
	if i := strings.IndexAny(s, " \t,}]"); i >= 0 {
		s = s[:i]
	}
	if s == "" || s == "|" || s == ">" {
		return "", false
	}
	return s, true
}

func yamlIndent(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

func stripYAMLComment(line string) string {
	return stripHashComment(line)
}

func stripTOMLComment(line string) string {
	return stripHashComment(line)
}

func stripHashComment(line string) string {
	var b strings.Builder
	inStr := byte(0)
	esc := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inStr != 0 {
			b.WriteByte(c)
			if esc {
				esc = false
				continue
			}
			if c == '\\' && inStr == '"' {
				esc = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			b.WriteByte(c)
			continue
		}
		if c == '#' {
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}

func parseTOMLStringArray(s string) []string {
	s = strings.TrimSpace(s)
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '"' {
			continue
		}
		j := i + 1
		var b strings.Builder
		for j < len(s) {
			if s[j] == '\\' && j+1 < len(s) {
				b.WriteByte(s[j+1])
				j += 2
				continue
			}
			if s[j] == '"' {
				break
			}
			b.WriteByte(s[j])
			j++
		}
		out = append(out, b.String())
		i = j
	}
	return out
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, io.ErrUnexpectedEOF
	}
	return b, nil
}
