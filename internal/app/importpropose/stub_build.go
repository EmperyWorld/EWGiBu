package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
)

func missingBuildContextDiags(workspace string, images map[string]any) []diagnostic.Diagnostic {
	if len(images) == 0 || strings.TrimSpace(workspace) == "" {
		return nil
	}
	var out []diagnostic.Diagnostic
	ids := sortedImageIDs(images)
	for _, id := range ids {
		ctx, ok := imageBuildContext(images[id])
		if !ok {
			continue
		}
		abs := filepath.Join(workspace, filepath.FromSlash(ctx))
		if _, err := os.Lstat(abs); err != nil {
			out = append(out, diagnostic.Warning(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("image %s: build context %q not found in workspace", id, ctx)).
				WithSource("", "/spec/images/"+id+"/build/context", 0, 0).
				WithHint("Use Create stub directories in the import panel or add the build context tree"))
			continue
		}
	}
	return out
}

func sortedImageIDs(images map[string]any) []string {
	ids := make([]string, 0, len(images))
	for id := range images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func imageBuildContext(raw any) (context string, ok bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	b, ok := m["build"].(map[string]any)
	if !ok {
		return "", false
	}
	c, _ := b["context"].(string)
	c = filepath.ToSlash(strings.TrimSpace(c))
	if c == "" || c == "." {
		return ".", true
	}
	return c, true
}
