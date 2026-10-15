package apply

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/provider"
)

var pgIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

type databaseEnsurer interface {
	EnsurePostgresDatabase(ctx context.Context, project, service, user, db string) error
}

// ensureServiceDatabase creates POSTGRES_DB when the server is already up but
// init was skipped. A data directory left by an interrupted first start has
// the role and no application database. Other names, including a second
// application database, are not created here.
func ensureServiceDatabase(ctx context.Context, p provider.ApplyOps, project, service string, dep ir.Deployment, deploy map[string]string) error {
	svc, ok := dep.Services[service]
	if !ok {
		return nil
	}
	user, db, ok := postgresIdentity(svc, deploy)
	if !ok || !postgresServerImage(dep, svc) {
		return nil
	}
	ensurer, ok := p.(databaseEnsurer)
	if !ok {
		return nil
	}
	if err := ensurer.EnsurePostgresDatabase(ctx, project, service, user, db); err != nil {
		return fmt.Errorf("service %q: create database %s: %w", service, db, err)
	}
	return nil
}

func postgresIdentity(svc ir.Service, deploy map[string]string) (user, db string, ok bool) {
	user, ok = plainEnv(svc.Environment["POSTGRES_USER"], deploy)
	if !ok || !pgIdent.MatchString(user) {
		return "", "", false
	}
	db, ok = plainEnv(svc.Environment["POSTGRES_DB"], deploy)
	if !ok || !pgIdent.MatchString(db) {
		return "", "", false
	}
	return user, db, true
}

// postgresServerImage is the database server, not a client that only has
// POSTGRES_USER and POSTGRES_DB in its environment. The API image does not
// contain psql.
func postgresServerImage(dep ir.Deployment, svc ir.Service) bool {
	img, ok := dep.Images[svc.Image]
	if !ok {
		return false
	}
	ref := strings.ToLower(strings.TrimSpace(img.Ref))
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	if i := strings.IndexAny(ref, ":@"); i >= 0 {
		ref = ref[:i]
	}
	return ref == "postgres"
}

func plainEnv(vs ir.ValueSource, deploy map[string]string) (string, bool) {
	if vs.Secret != "" || len(vs.Parts) > 0 {
		return "", false
	}
	if vs.Parameter != "" {
		v, ok := deploy[vs.Parameter]
		return v, ok && pgIdent.MatchString(v)
	}
	s, ok := vs.Literal.(string)
	return s, ok && s != ""
}
