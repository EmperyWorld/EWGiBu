package portable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/domain/ir"
)

func hostParamID(service, target string) string {
	raw := strings.ToLower(service + "-" + strings.Trim(target, "/"))
	var b strings.Builder
	prevDash := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "path"
	}
	return "host-" + s
}

func absoluteHostPath(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, "\n\r\x00") {
		return false
	}
	return filepath.IsAbs(v)
}

// ApplyKitHostPaths replaces host binds in dep with filled kit values.
// preserve lists deploy parameters the operator already set; those mounts
// stay on the parameter so --set wins.
func ApplyKitHostPaths(dep *ir.Deployment, kitDir string, preserve map[string]string) {
	if dep == nil || kitDir == "" {
		return
	}
	fills := kitHostFills(kitDir)
	if len(fills) == 0 {
		return
	}
	for id, svc := range dep.Services {
		svc.Mounts = fillMounts(id, svc.Mounts, fills, preserve)
		dep.Services[id] = svc
	}
	for id, task := range dep.Tasks {
		task.Mounts = fillMounts(id, task.Mounts, fills, preserve)
		dep.Tasks[id] = task
	}
}

// KitHostStamp is a digest of filled host paths. Apply stores it so a later
// run with the same bundle and the same paths can noop, and a changed path
// cannot.
func KitHostStamp(kitDir string) string {
	if kitDir == "" {
		return ""
	}
	fills := kitHostFills(kitDir)
	keys := make([]string, 0, len(fills))
	for k := range fills {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, map[string]string{"mount": k, "value": fills[k]})
	}
	raw, err := canonicaljson.Marshal(rows)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func kitHostFills(kitDir string) map[string]string {
	raw, err := os.ReadFile(filepath.Join(kitDir, "host-paths.json"))
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, row := range rows {
		svc, _ := row["service"].(string)
		tgt, _ := row["target"].(string)
		val, _ := row["value"].(string)
		if svc == "" || tgt == "" || !absoluteHostPath(val) {
			continue
		}
		out[svc+"\x00"+tgt] = strings.TrimSpace(val)
	}
	return out
}

func fillMounts(id string, mounts []ir.Mount, fills, preserve map[string]string) []ir.Mount {
	for i := range mounts {
		m := &mounts[i]
		if m.HostLiteral == "" && m.HostParameter == "" {
			continue
		}
		if m.HostParameter != "" {
			if _, keep := preserve[m.HostParameter]; keep {
				continue
			}
		}
		v, ok := fills[id+"\x00"+m.Target]
		if !ok {
			continue
		}
		m.HostLiteral = v
		m.HostParameter = ""
	}
	return mounts
}
