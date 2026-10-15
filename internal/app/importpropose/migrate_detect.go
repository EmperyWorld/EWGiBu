package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
)

// migrateHit is a workspace migrations tree for propose reporting.
type migrateHit struct {
	Root         string // workspace-relative, slash form
	HasBaseline  bool
	HasPending   bool
	PendingCount int
}

func scanMigrationTrees(workspace string) []migrateHit {
	root := filepath.Clean(workspace)
	var hits []migrateHit
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, "/")
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "applied" {
				return filepath.SkipDir
			}
			if depth > 6 {
				return filepath.SkipDir
			}
			if base != "migrations" {
				return nil
			}
			hit := migrateHit{Root: rel}
			baseSQL := filepath.Join(path, "baseline.sql")
			if st, err := os.Stat(baseSQL); err == nil && !st.IsDir() {
				hit.HasBaseline = true
			}
			pending := filepath.Join(path, "pending")
			if st, err := os.Stat(pending); err == nil && st.IsDir() {
				hit.HasPending = true
				entries, _ := os.ReadDir(pending)
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
						hit.PendingCount++
					}
				}
			}
			if hit.HasBaseline || hit.HasPending {
				hits = append(hits, hit)
			}
			return filepath.SkipDir
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool { return hits[i].Root < hits[j].Root })
	return hits
}

func pickPostgresServiceID(services map[string]any) string {
	ids := make([]string, 0, len(services))
	for id := range services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "postgres" || strings.Contains(id, "postgres") {
			return id
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return "postgres"
}

func migrateGapDiagnostics(workspace string, services map[string]any) []diagnostic.Diagnostic {
	hits := scanMigrationTrees(workspace)
	if len(hits) == 0 {
		return nil
	}
	var diags []diagnostic.Diagnostic
	pg := pickPostgresServiceID(services)
	for _, h := range hits {
		taskID := "sql-migrate"
		msg := fmt.Sprintf(
			"migrations tree %q detected (baseline=%v pendingSQL=%d); Compose import does not create spec.tasks — add a migration task and service dependsOn completedSuccessfully",
			h.Root, h.HasBaseline, h.PendingCount,
		)
		hint := migrateTaskTemplate(h.Root, taskID, pg, h.HasBaseline)
		if !h.HasBaseline {
			hint = "Missing baseline.sql (greenfield requires one full-schema dump; applied/ is history and is not replayed). " + hint
		}
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportMigrateGap, msg).
			WithSource(h.Root, "", 0, 0).
			WithHint(hint))
	}
	return diags
}

func migrateTaskTemplate(migRoot, taskID, postgresID string, hasBaseline bool) string {
	baseNote := "baseline.sql + pending/*.sql"
	if !hasBaseline {
		baseNote = "add baseline.sql then pending/*.sql"
	}
	return fmt.Sprintf(`Example (argv only; mount %s; %s; tracking table in DB; applied/ ignored):

spec:
  resources:
    res-migrations:
      source: %s
      include: ["**"]
      allowEmpty: true
  tasks:
    %s:
      phase: migration
      image: <postgres-image-id>
      command:
        - /migrations/pack-migrate
      dependsOn:
        - target: %s
          condition: healthy
      mounts:
        - resource: res-migrations
          target: /migrations
          readOnly: true
      secrets:
        - ref: postgres-password
          target: /run/secrets/postgres-password
      rerun: oncePerDeployment
      timeout: 30m
  services:
    <api-service>:
      dependsOn:
        - target: %s
          condition: completedSuccessfully

Ship pack-migrate as an executable in the migrations tree (see docs/release/examples/pack-migrate).`,
		migRoot, baseNote, migRoot, taskID, postgresID, taskID)
}
