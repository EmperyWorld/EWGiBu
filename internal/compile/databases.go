package compile

import (
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
)

// expandDatabases adds one migration task per spec.databases entry and
// rewrites urlParameter references into a DSN. The password secret is a
// reference, never a value. No second database server is created.
func expandDatabases(d *ir.Deployment, p domain.Project) error {
	if len(p.Spec.Databases) == 0 {
		return nil
	}
	ids := make([]string, 0, len(p.Spec.Databases))
	for id := range p.Spec.Databases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if d.Tasks == nil {
		d.Tasks = map[string]ir.Task{}
	}
	for _, id := range ids {
		db := p.Spec.Databases[id]
		on, ok := p.Spec.Services[db.On]
		if !ok {
			return fmt.Errorf("database %q on unknown service %q", id, db.On)
		}
		owner, ok := p.Spec.Services[db.OwnerFrom]
		if !ok {
			return fmt.Errorf("database %q ownerFrom unknown service %q", id, db.OwnerFrom)
		}
		taskID := "db-" + id
		if _, exists := d.Tasks[taskID]; exists {
			return fmt.Errorf("database %q task %q already exists", id, taskID)
		}
		userLit, userParam, secretID := postgresAccount(owner)
		parts := dsnParts(userLit, userParam, secretID, db.On, db.Name)
		rewriteURLParameter(d, db.URLParameter, parts)
		taskUser := on.User
		if !containerUserNumeric(taskUser) {
			// The one-shot client must read the secret file. A non-numeric
			// image user cannot, and rootless Podman shows the host owner as uid 0.
			taskUser = "0"
		}
		task := ir.Task{
			Phase:      "migration",
			Image:      on.Image,
			User:       taskUser,
			Command:    ir.LiteralArgs("/iacpack/db/migrate.sh"),
			Entrypoint: ir.LiteralArgs("/bin/sh"),
			Rerun:      "oncePerDeployment",
			Mounts: []ir.Mount{{
				Resource: taskID, Target: "/iacpack/db", ReadOnly: true,
			}},
			Networks: append([]string(nil), on.Networks...),
			DependsOn: []ir.Dependency{{
				Target: db.On, Condition: "healthy",
			}},
		}
		if secretID != "" {
			task.Secrets = []ir.SecretMount{{Ref: secretID, Target: "/run/secrets/db"}}
		}
		if userParam != "" {
			task.Environment = map[string]ir.ValueSource{
				"PGUSER": {Parameter: userParam},
			}
		}
		d.Tasks[taskID] = task
	}
	return nil
}

func postgresAccount(svc domain.Service) (userLit, userParam, secretID string) {
	if vs, ok := svc.Environment["POSTGRES_USER"]; ok {
		if vs.Literal != nil && vs.Literal.Kind == domain.ScalarString {
			userLit = vs.Literal.Str
		}
		userParam = vs.Parameter
	}
	if vs, ok := svc.Environment["POSTGRES_PASSWORD"]; ok {
		secretID = vs.Secret
	}
	if secretID == "" {
		for _, sm := range svc.Secrets {
			if sm.Ref != "" {
				secretID = sm.Ref
				break
			}
		}
	}
	if userLit == "" && userParam == "" {
		userLit = "postgres"
	}
	return userLit, userParam, secretID
}

func containerUserNumeric(user string) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return false
	}
	if strings.Contains(user, ":") {
		parts := strings.Split(user, ":")
		if len(parts) != 2 {
			return false
		}
		return digits(parts[0]) && digits(parts[1])
	}
	return digits(user)
}

func digits(s string) bool {
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

func dsnParts(userLit, userParam, secretID, host, name string) []ir.ValueSource {
	var parts []ir.ValueSource
	parts = append(parts, ir.ValueSource{Literal: "postgresql://"})
	if userParam != "" {
		parts = append(parts, ir.ValueSource{Parameter: userParam})
	} else {
		parts = append(parts, ir.ValueSource{Literal: userLit})
	}
	if secretID != "" {
		parts = append(parts, ir.ValueSource{Literal: ":"}, ir.ValueSource{Secret: secretID})
	}
	parts = append(parts, ir.ValueSource{Literal: "@" + host + ":5432/" + name})
	return parts
}

// DatabaseMigrateScript is the idempotent shell for one extra database.
// It creates the database when missing and applies SQL files in name order.
// The password is read from /run/secrets/db and is not embedded.
func DatabaseMigrateScript(userLit, host, name string, userFromEnv, hasSecret bool) string {
	userLine := "PGUSER='" + sqlQuote(userLit) + "'"
	if userFromEnv {
		userLine = `PGUSER="${PGUSER:-}"`
	}
	secretLine := ""
	if hasSecret {
		secretLine = `
if [ -f /run/secrets/db ]; then
  PGPASSWORD=$(cat /run/secrets/db)
  export PGPASSWORD
fi`
	}
	return `#!/bin/sh
set -eu
` + userLine + `
export PGUSER
export PGHOST='` + sqlQuote(host) + `'
DB='` + sqlQuote(name) + `'
` + secretLine + `
exists=$(psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='${DB}'" || true)
if [ "$exists" != "1" ]; then
  psql -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"${DB}\""
fi
psql -d "$DB" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS iacpack_schema_migrations (name text PRIMARY KEY, hash text NOT NULL DEFAULT '')"
psql -d "$DB" -v ON_ERROR_STOP=1 -c "ALTER TABLE iacpack_schema_migrations ADD COLUMN IF NOT EXISTS hash text NOT NULL DEFAULT ''"
for f in $(find /iacpack/db/sql -type f -name '*.sql' | sort); do
  base=$(basename "$f")
  file_hash=$(sha256sum "$f" | awk '{print $1}')
  applied=$(psql -d "$DB" -tAc "SELECT hash FROM iacpack_schema_migrations WHERE name='${base}'")
  applied=$(printf '%s' "$applied" | tr -d '[:space:]')
  if [ "$applied" = "$file_hash" ]; then
    continue
  fi
  psql -d "$DB" -v ON_ERROR_STOP=1 -f "$f"
  psql -d "$DB" -v ON_ERROR_STOP=1 -c "INSERT INTO iacpack_schema_migrations (name, hash) VALUES ('${base}', '${file_hash}') ON CONFLICT (name) DO UPDATE SET hash = EXCLUDED.hash"
done
`
}

func sqlQuote(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func rewriteURLParameter(d *ir.Deployment, paramID string, parts []ir.ValueSource) {
	if paramID == "" {
		return
	}
	for id, svc := range d.Services {
		if len(svc.Environment) == 0 {
			continue
		}
		changed := false
		for k, vs := range svc.Environment {
			if vs.Parameter == paramID && len(vs.Parts) == 0 && vs.Secret == "" {
				svc.Environment[k] = ir.ValueSource{Parts: parts}
				changed = true
			}
		}
		if changed {
			d.Services[id] = svc
		}
	}
}
