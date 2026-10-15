package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/bundle"
	"iacpack/internal/canonicaljson"
	"iacpack/internal/domain/ir"
)

const taskInputsVersion = "iacpack.taskinputs/v1"

type taskInputsDoc struct {
	FormatVersion string            `json:"formatVersion"`
	Tasks         map[string]string `json:"tasks"`
}

func loadTaskInputs(path string) map[string]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var doc taskInputsDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Tasks == nil {
		return map[string]string{}
	}
	return doc.Tasks
}

func saveTaskInputs(path string, tasks map[string]string) error {
	doc := taskInputsDoc{FormatVersion: taskInputsVersion, Tasks: tasks}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// migrationInputDrift is true when a phase:migration oncePerDeployment task
// has a different command, entrypoint, user, or mount-file hash than the last
// successful run.
func migrationInputDrift(dep ir.Deployment, entries map[string]bundle.Entry, deploy map[string]string, prev map[string]string) bool {
	for _, id := range sortedTaskIDs(dep) {
		task := dep.Tasks[id]
		if task.Rerun != "oncePerDeployment" {
			continue
		}
		if prev[id] != migrationInputHash(task, entries, deploy) {
			return true
		}
	}
	return false
}

func migrationInputHash(task ir.Task, entries map[string]bundle.Entry, deploy map[string]string) string {
	mounts := make([]map[string]string, 0, len(task.Mounts))
	indexed := append([]ir.Mount(nil), task.Mounts...)
	sort.Slice(indexed, func(i, j int) bool {
		if indexed[i].Target == indexed[j].Target {
			return mountKind(indexed[i]) < mountKind(indexed[j])
		}
		return indexed[i].Target < indexed[j].Target
	})
	for _, m := range indexed {
		item := map[string]string{
			"target":   m.Target,
			"readOnly": boolStr(m.ReadOnly),
			"kind":     mountKind(m),
			"files":    mountFileHash(m, entries, deploy),
		}
		mounts = append(mounts, item)
	}
	body := map[string]any{
		"command":    task.Command,
		"entrypoint": task.Entrypoint,
		"user":       task.User,
		"mounts":     mounts,
	}
	raw, err := canonicaljson.Marshal(body)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func mountKind(m ir.Mount) string {
	switch {
	case m.Volume != "":
		return "volume:" + m.Volume
	case m.Resource != "":
		return "resource:" + m.Resource
	case m.Build != "":
		return "build:" + m.Build
	case m.HostParameter != "":
		return "host-parameter:" + m.HostParameter
	case m.HostLiteral != "":
		return "host:" + m.HostLiteral
	default:
		return ""
	}
}

func mountFileHash(m ir.Mount, entries map[string]bundle.Entry, deploy map[string]string) string {
	switch {
	case m.Resource != "":
		return payloadHash(entries, "payload/"+m.Resource+"/")
	case m.Build != "":
		return payloadHash(entries, "payload/build-"+m.Build+"/")
	case m.HostLiteral != "" || m.HostParameter != "":
		src := m.HostLiteral
		if m.HostParameter != "" {
			src = deploy[m.HostParameter]
		}
		if src == "" {
			return "absent"
		}
		return hostTreeHash(src)
	case m.Volume != "":
		return "volume"
	default:
		return ""
	}
}

func payloadHash(entries map[string]bundle.Entry, prefix string) string {
	var lines []string
	for p, e := range entries {
		if !strings.HasPrefix(p, prefix) || strings.HasSuffix(p, "/") {
			continue
		}
		sum := e.SHA256
		if sum == "" && e.Content != nil {
			h := sha256.Sum256(e.Content)
			sum = hex.EncodeToString(h[:])
		}
		lines = append(lines, strings.TrimPrefix(p, prefix)+" "+sum)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
}

func hostTreeHash(root string) string {
	info, err := os.Lstat(root)
	if err != nil {
		return "absent"
	}
	if !info.IsDir() {
		if info.Mode()&os.ModeSymlink != 0 {
			return "symlink"
		}
		return fileHash(root)
	}
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		lines = append(lines, filepath.ToSlash(rel)+" "+fileHash(p))
		return nil
	})
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unreadable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func sortedTaskIDs(dep ir.Deployment) []string {
	ids := make([]string, 0, len(dep.Tasks))
	for id := range dep.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
