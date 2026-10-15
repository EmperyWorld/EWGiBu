package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"iacpack/internal/domain/ir"
)

// RequireSecretFiles checks that every Compose file-secret exists in the
// private secrets directory before a container starts. Compose mounts
// ../secrets/<id> relative to the rendered compose file.
func RequireSecretFiles(secretsDir string, ids []string) error {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	seen := map[string]struct{}{}
	for _, id := range sorted {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		st, err := os.Stat(filepath.Join(secretsDir, id))
		if err != nil || st.IsDir() {
			return fmt.Errorf("secret %s is not readable", id)
		}
	}
	return nil
}

// TaskSecretIDs lists file-secret ids mounted by a task.
func TaskSecretIDs(dep ir.Deployment, taskID string) []string {
	task, ok := dep.Tasks[taskID]
	if !ok {
		return nil
	}
	var ids []string
	for _, s := range task.Secrets {
		ids = append(ids, s.Ref)
	}
	return ids
}

// ServiceSecretIDs lists file-secret ids mounted by a service.
func ServiceSecretIDs(dep ir.Deployment, serviceID string) []string {
	svc, ok := dep.Services[serviceID]
	if !ok {
		return nil
	}
	var ids []string
	for _, s := range svc.Secrets {
		ids = append(ids, s.Ref)
	}
	return ids
}
