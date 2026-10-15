package v1alpha1

import (
	"fmt"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/path"
	"iacpack/internal/manifest/loader"
)

func decodeDatabases(file string, node *loader.Node) (map[string]domain.Database, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/databases", "object", node)}
	}
	out := map[string]domain.Database{}
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/databases/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("database id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{
			"on": {}, "name": {}, "ownerFrom": {}, "migrations": {}, "urlParameter": {},
		})...)
		on, d := requireString(file, e.Value, "on", ptr+"/on")
		diags = append(diags, d...)
		name, d := requireString(file, e.Value, "name", ptr+"/name")
		diags = append(diags, d...)
		owner, d := requireString(file, e.Value, "ownerFrom", ptr+"/ownerFrom")
		diags = append(diags, d...)
		mig, d := requireString(file, e.Value, "migrations", ptr+"/migrations")
		diags = append(diags, d...)
		urlp, d := requireString(file, e.Value, "urlParameter", ptr+"/urlParameter")
		diags = append(diags, d...)
		if on != "" && !identifierRe.MatchString(on) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier, "database on must be a service id").
				WithSource(file, ptr+"/on", 0, 0))
		}
		if owner != "" && !identifierRe.MatchString(owner) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier, "database ownerFrom must be a service id").
				WithSource(file, ptr+"/ownerFrom", 0, 0))
		}
		if urlp != "" && !identifierRe.MatchString(urlp) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier, "database urlParameter must be a parameter id").
				WithSource(file, ptr+"/urlParameter", 0, 0))
		}
		if name != "" && !dbNameRe.MatchString(name) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"database name must be a SQL identifier").WithSource(file, ptr+"/name", 0, 0))
		}
		if mig != "" {
			if _, err := path.ParseWorkspacePath(mig); err != nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()).
					WithSource(file, ptr+"/migrations", 0, 0))
			}
		}
		out[id] = domain.Database{On: on, Name: name, OwnerFrom: owner, Migrations: mig, URLParameter: urlp}
	}
	if len(out) == 0 {
		return nil, diags
	}
	return out, diags
}
