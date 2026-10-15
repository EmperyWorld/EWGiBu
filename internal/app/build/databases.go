package build

import (
	"sort"

	"iacpack/internal/artifact"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
)

func addDatabaseScripts(tree *artifact.Tree, project domain.Project) []diagnostic.Diagnostic {
	ids := make([]string, 0, len(project.Spec.Databases))
	for id := range project.Spec.Databases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var diags []diagnostic.Diagnostic
	for _, id := range ids {
		db := project.Spec.Databases[id]
		owner := project.Spec.Services[db.OwnerFrom]
		userLit, userParam, secretID := account(owner)
		script := compile.DatabaseMigrateScript(userLit, db.On, db.Name, userParam != "", secretID != "")
		dest := "payload/db-" + id + "/migrate.sh"
		if err := tree.AddFile(dest, "0755", []byte(script)); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		}
	}
	return diags
}

func account(svc domain.Service) (userLit, userParam, secretID string) {
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
