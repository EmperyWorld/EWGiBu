package v1alpha1

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/path"
	"iacpack/internal/manifest/loader"
)

func decodePackageBuilds(file string, node *loader.Node) (map[string]domain.PackageBuild, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/builds", "object", node)}
	}
	out := make(map[string]domain.PackageBuild, len(node.Map))
	managers := map[string]struct{}{"pnpm": {}, "npm": {}, "yarn": {}, "bun": {}}
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/builds/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("build id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{
			"context": {}, "packageManager": {}, "install": {}, "script": {}, "output": {}, "entry": {},
		})...)
		var b domain.PackageBuild
		ctx, d := requireString(file, e.Value, "context", ptr+"/context")
		diags = append(diags, d...)
		if ctx != "" {
			wp, err := path.ParseWorkspacePath(ctx)
			if err != nil {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
					file, ptr+"/context", e.Value.GetMap("context")))
			} else {
				b.Context = wp
			}
		}
		mgr, d := requireString(file, e.Value, "packageManager", ptr+"/packageManager")
		diags = append(diags, d...)
		if mgr != "" {
			if _, ok := managers[mgr]; !ok {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"packageManager must be pnpm, npm, yarn, or bun"), file, ptr+"/packageManager", e.Value.GetMap("packageManager")))
			}
			b.PackageManager = mgr
		}
		inst, d := requireString(file, e.Value, "install", ptr+"/install")
		diags = append(diags, d...)
		if inst != "" && inst != "frozen" {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"install must be frozen"), file, ptr+"/install", e.Value.GetMap("install")))
		}
		b.Install = inst
		script, d := requireString(file, e.Value, "script", ptr+"/script")
		diags = append(diags, d...)
		if script != "" && !scriptNameRe.MatchString(script) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"script must be a package.json script name"), file, ptr+"/script", e.Value.GetMap("script")))
		}
		b.Script = script
		output, d := requireString(file, e.Value, "output", ptr+"/output")
		diags = append(diags, d...)
		if output != "" && !validOutputRel(output) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath,
				"output must be a relative path inside the context"), file, ptr+"/output", e.Value.GetMap("output")))
		}
		b.Output = output
		b.Entry = "index.html"
		if ent := e.Value.GetMap("entry"); ent != nil {
			if ent.Kind != loader.KindString || !validOutputRel(ent.Str) {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath,
					"entry must be a relative file path"), file, ptr+"/entry", ent))
			} else {
				b.Entry = ent.Str
			}
		}
		out[id] = b
	}
	return out, diags
}

func validOutputRel(s string) bool {
	if s == "" || strings.HasPrefix(s, "/") || strings.Contains(s, "\\") || strings.Contains(s, "..") {
		return false
	}
	return true
}

func decodeResources(file string, node *loader.Node) (map[string]domain.Resource, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/resources", "object", node)}
	}
	out := make(map[string]domain.Resource, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/resources/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("resource id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{
			"source": {}, "include": {}, "exclude": {}, "destination": {}, "mode": {}, "allowEmpty": {},
		})...)
		var r domain.Resource
		src, d := requireString(file, e.Value, "source", ptr+"/source")
		diags = append(diags, d...)
		if src != "" {
			wp, err := path.ParseWorkspacePath(src)
			if err != nil {
				n := e.Value.GetMap("source")
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
					file, ptr+"/source", n))
			} else {
				r.Source = wp
			}
		}
		inc := e.Value.GetMap("include")
		if inc == nil {
			diags = append(diags, missing(file, ptr+"/include", e.Value))
		} else if inc.Kind != loader.KindSeq || len(inc.Seq) == 0 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"resource.include must be a non-empty array"), file, ptr+"/include", inc))
		} else {
			for i, item := range inc.Seq {
				if item == nil || item.Kind != loader.KindString || item.Str == "" {
					diags = append(diags, typeErr(file, fmt.Sprintf("%s/include/%d", ptr, i), "non-empty string", item))
					continue
				}
				r.Include = append(r.Include, item.Str)
			}
		}
		if excl := e.Value.GetMap("exclude"); excl != nil {
			if excl.Kind != loader.KindSeq {
				diags = append(diags, typeErr(file, ptr+"/exclude", "array", excl))
			} else {
				for i, item := range excl.Seq {
					if item == nil || item.Kind != loader.KindString || item.Str == "" {
						diags = append(diags, typeErr(file, fmt.Sprintf("%s/exclude/%d", ptr, i), "non-empty string", item))
						continue
					}
					r.Exclude = append(r.Exclude, item.Str)
				}
			}
		}
		if dest := e.Value.GetMap("destination"); dest != nil {
			if dest.Kind != loader.KindString || dest.Str == "" {
				diags = append(diags, typeErr(file, ptr+"/destination", "non-empty string", dest))
			} else if bp, err := path.ParseBundlePath(dest.Str); err != nil {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
					file, ptr+"/destination", dest))
			} else {
				r.Destination = bp
			}
		}
		if mode := e.Value.GetMap("mode"); mode != nil {
			if mode.Kind != loader.KindString || !modeOctalRe.MatchString(mode.Str) {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					`resource.mode must match ^0[0-7]{3}$`), file, ptr+"/mode", mode))
			} else {
				r.Mode = mode.Str
			}
		}
		if ae := e.Value.GetMap("allowEmpty"); ae != nil {
			if ae.Kind != loader.KindBool {
				diags = append(diags, typeErr(file, ptr+"/allowEmpty", "boolean", ae))
			} else {
				r.AllowEmpty = ae.Bool
			}
		}
		out[id] = r
	}
	return out, diags
}

func decodeNetworks(file string, node *loader.Node) (map[string]domain.Network, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/networks", "object", node)}
	}
	out := make(map[string]domain.Network, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/networks/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("network id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind == loader.KindNull {
			out[id] = domain.Network{}
			continue
		}
		if e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		ext := e.Value.GetMap("external")
		if ext != nil {
			diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{"external": {}})...)
			ref, d := decodeExternal(file, ptr+"/external", ext)
			diags = append(diags, d...)
			out[id] = domain.Network{External: &ref}
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{})...)
		out[id] = domain.Network{}
	}
	return out, diags
}

func decodeVolumes(file string, node *loader.Node) (map[string]domain.Volume, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/volumes", "object", node)}
	}
	out := make(map[string]domain.Volume, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/volumes/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("volume id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		ext := e.Value.GetMap("external")
		if ext != nil {
			diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{"external": {}})...)
			ref, d := decodeExternal(file, ptr+"/external", ext)
			diags = append(diags, d...)
			out[id] = domain.Volume{External: &ref}
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{"retention": {}})...)
		vol := domain.Volume{Retention: "retain"}
		if ret := e.Value.GetMap("retention"); ret != nil {
			if ret.Kind != loader.KindString || (ret.Str != "retain" && ret.Str != "delete") {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					`volume.retention must be "retain" or "delete"`), file, ptr+"/retention", ret))
			} else {
				vol.Retention = ret.Str
			}
		}
		out[id] = vol
	}
	return out, diags
}

func decodeExternal(file, ptr string, node *loader.Node) (domain.ExternalRef, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var ref domain.ExternalRef
	if node == nil || node.Kind != loader.KindMap {
		return ref, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{"name": {}})...)
	name, d := requireString(file, node, "name", ptr+"/name")
	diags = append(diags, d...)
	ref.Name = name
	return ref, diags
}

func validServiceUser(s string) bool {
	if s == "" || len(s) > 64 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-' || r == ':':
		default:
			return false
		}
	}
	return true
}

func decodeServices(file string, node *loader.Node) (map[string]domain.Service, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node == nil {
		return nil, []diagnostic.Diagnostic{missing(file, "/spec/services", nil)}
	}
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/services", "object", node)}
	}
	if len(node.Map) == 0 {
		return nil, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaMissingField,
			"spec.services must contain at least one service"), file, "/spec/services", node)}
	}
	if len(node.Map) > 500 {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"spec.services exceeds maximum of 500"), file, "/spec/services", node))
	}
	out := make(map[string]domain.Service, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/services/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("service id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		svc, d := decodeService(file, ptr, e.Value)
		diags = append(diags, d...)
		out[id] = svc
	}
	return out, diags
}

func decodeService(file, ptr string, node *loader.Node) (domain.Service, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var svc domain.Service
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"image": {}, "user": {}, "command": {}, "entrypoint": {}, "environment": {}, "environmentFrom": {}, "secrets": {},
		"ports": {}, "mounts": {}, "networks": {}, "healthcheck": {}, "restart": {},
		"resources": {}, "dependsOn": {}, "profiles": {},
	})...)
	img, d := requireString(file, node, "image", ptr+"/image")
	diags = append(diags, d...)
	svc.Image = img
	if u := node.GetMap("user"); u != nil {
		if u.Kind != loader.KindString || !validServiceUser(u.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"user must be a user name or uid:gid"), file, ptr+"/user", u))
		} else {
			svc.User = u.Str
		}
	}
	if cmd := node.GetMap("command"); cmd != nil {
		argv, d := decodeCommand(file, ptr+"/command", cmd)
		diags = append(diags, d...)
		svc.Command = argv
	}
	if ep := node.GetMap("entrypoint"); ep != nil {
		argv, d := decodeCommand(file, ptr+"/entrypoint", ep)
		diags = append(diags, d...)
		svc.Entrypoint = argv
	}
	if env := node.GetMap("environment"); env != nil {
		m, d := decodeEnvMap(file, ptr+"/environment", env)
		diags = append(diags, d...)
		svc.Environment = m
	}
	if from := node.GetMap("environmentFrom"); from != nil {
		if from.Kind != loader.KindString || !identifierRe.MatchString(from.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"environmentFrom must be a service id"), file, ptr+"/environmentFrom", from))
		} else {
			svc.EnvironmentFrom = from.Str
		}
	}
	if secs := node.GetMap("secrets"); secs != nil {
		sm, d := decodeSecretMounts(file, ptr+"/secrets", secs)
		diags = append(diags, d...)
		svc.Secrets = sm
	}
	if ports := node.GetMap("ports"); ports != nil {
		ps, d := decodePorts(file, ptr+"/ports", ports)
		diags = append(diags, d...)
		svc.Ports = ps
	}
	if mounts := node.GetMap("mounts"); mounts != nil {
		ms, d := decodeMounts(file, ptr+"/mounts", mounts)
		diags = append(diags, d...)
		svc.Mounts = ms
	}
	if nets := node.GetMap("networks"); nets != nil {
		ids, d := decodeIDList(file, ptr+"/networks", nets)
		diags = append(diags, d...)
		svc.Networks = ids
	}
	if profs := node.GetMap("profiles"); profs != nil {
		names, d := decodeProfileNames(file, ptr+"/profiles", profs)
		diags = append(diags, d...)
		svc.Profiles = names
	}
	if hc := node.GetMap("healthcheck"); hc != nil {
		h, d := decodeHealthcheck(file, ptr+"/healthcheck", hc)
		diags = append(diags, d...)
		svc.Healthcheck = &h
	}
	svc.Restart = "unless-stopped"
	if rs := node.GetMap("restart"); rs != nil {
		if rs.Kind != loader.KindString {
			diags = append(diags, typeErr(file, ptr+"/restart", "string", rs))
		} else {
			switch rs.Str {
			case "no", "always", "on-failure", "unless-stopped":
				svc.Restart = rs.Str
			default:
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					fmt.Sprintf("unsupported restart policy %q", rs.Str)), file, ptr+"/restart", rs))
			}
		}
	}
	if res := node.GetMap("resources"); res != nil {
		rl, d := decodeResourceLimits(file, ptr+"/resources", res)
		diags = append(diags, d...)
		svc.Resources = &rl
	}
	if deps := node.GetMap("dependsOn"); deps != nil {
		ds, d := decodeDependencies(file, ptr+"/dependsOn", deps)
		diags = append(diags, d...)
		svc.DependsOn = ds
	}
	return svc, diags
}

func decodeTasks(file string, node *loader.Node) (map[string]domain.Task, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/tasks", "object", node)}
	}
	if len(node.Map) > 500 {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"spec.tasks exceeds maximum of 500"), file, "/spec/tasks", node))
	}
	out := make(map[string]domain.Task, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/tasks/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("task id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		t, d := decodeTask(file, ptr, e.Value)
		diags = append(diags, d...)
		out[id] = t
	}
	return out, diags
}

func decodeTask(file, ptr string, node *loader.Node) (domain.Task, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var t domain.Task
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"phase": {}, "image": {}, "command": {}, "entrypoint": {}, "user": {}, "workdir": {}, "dependsOn": {},
		"timeout": {}, "retry": {}, "rerun": {}, "mounts": {}, "networks": {}, "secrets": {},
	})...)
	phase, d := requireString(file, node, "phase", ptr+"/phase")
	diags = append(diags, d...)
	switch phase {
	case "preStart", "migration", "postDeploy":
		t.Phase = domain.TaskPhase(phase)
	case "":
	default:
		n := node.GetMap("phase")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			fmt.Sprintf("unsupported task phase %q", phase)), file, ptr+"/phase", n))
	}
	img, d := requireString(file, node, "image", ptr+"/image")
	diags = append(diags, d...)
	t.Image = img
	cmdNode := node.GetMap("command")
	if cmdNode == nil {
		diags = append(diags, missing(file, ptr+"/command", node))
	} else {
		argv, d := decodeCommand(file, ptr+"/command", cmdNode)
		diags = append(diags, d...)
		t.Command = argv
	}
	if ep := node.GetMap("entrypoint"); ep != nil {
		if ep.Kind == loader.KindSeq && len(ep.Seq) == 0 {
			t.ResetEntrypoint = true
		} else {
			argv, d := decodeCommand(file, ptr+"/entrypoint", ep)
			diags = append(diags, d...)
			t.Entrypoint = argv
		}
	}
	if u := node.GetMap("user"); u != nil {
		if u.Kind != loader.KindString || !validServiceUser(u.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"user must be a uid, uid:gid, or user name"), file, ptr+"/user", u))
		} else {
			t.User = u.Str
		}
	}
	if wd := node.GetMap("workdir"); wd != nil {
		if wd.Kind != loader.KindString {
			diags = append(diags, typeErr(file, ptr+"/workdir", "string", wd))
		} else if cp, err := path.ParseContainerPath(wd.Str); err != nil {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
				file, ptr+"/workdir", wd))
		} else {
			t.Workdir = cp
		}
	}
	if deps := node.GetMap("dependsOn"); deps != nil {
		ds, d := decodeDependencies(file, ptr+"/dependsOn", deps)
		diags = append(diags, d...)
		t.DependsOn = ds
	}
	if to := node.GetMap("timeout"); to != nil {
		dur, d := decodeDuration(file, ptr+"/timeout", to)
		diags = append(diags, d...)
		t.Timeout = dur
	}
	if ry := node.GetMap("retry"); ry != nil {
		r, d := decodeRetry(file, ptr+"/retry", ry)
		diags = append(diags, d...)
		t.Retry = &r
	}
	t.Rerun = "oncePerDeployment"
	if rr := node.GetMap("rerun"); rr != nil {
		if rr.Kind != loader.KindString {
			diags = append(diags, typeErr(file, ptr+"/rerun", "string", rr))
		} else {
			switch rr.Str {
			case "always", "oncePerDeployment", "manual":
				t.Rerun = rr.Str
			default:
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					fmt.Sprintf("unsupported rerun %q", rr.Str)), file, ptr+"/rerun", rr))
			}
		}
	}
	if mounts := node.GetMap("mounts"); mounts != nil {
		ms, d := decodeMounts(file, ptr+"/mounts", mounts)
		diags = append(diags, d...)
		t.Mounts = ms
	}
	if nets := node.GetMap("networks"); nets != nil {
		ids, d := decodeIDList(file, ptr+"/networks", nets)
		diags = append(diags, d...)
		t.Networks = ids
	}
	if secs := node.GetMap("secrets"); secs != nil {
		sm, d := decodeSecretMounts(file, ptr+"/secrets", secs)
		diags = append(diags, d...)
		t.Secrets = sm
	}
	return t, diags
}

func decodePackage(file string, node *loader.Node) (domain.Package, []diagnostic.Diagnostic) {
	pkg := domain.Package{
		GenerateReadme:     true,
		GenerateEnvExample: true,
		IncludeRuntime:     false,
		EmbedImages:        false,
	}
	if node == nil {
		return pkg, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return pkg, []diagnostic.Diagnostic{typeErr(file, "/spec/package", "object", node)}
	}
	pkg.Set = true
	diags = append(diags, rejectUnknown(file, "/spec/package", node, map[string]struct{}{
		"generateReadme": {}, "generateEnvExample": {}, "includeRuntime": {}, "embedImages": {},
	})...)
	if v, d := optionalBool(file, "/spec/package/generateReadme", node.GetMap("generateReadme")); d != nil {
		diags = append(diags, d...)
	} else if node.GetMap("generateReadme") != nil {
		pkg.GenerateReadme = v
	}
	if v, d := optionalBool(file, "/spec/package/generateEnvExample", node.GetMap("generateEnvExample")); d != nil {
		diags = append(diags, d...)
	} else if node.GetMap("generateEnvExample") != nil {
		pkg.GenerateEnvExample = v
	}
	if v, d := optionalBool(file, "/spec/package/includeRuntime", node.GetMap("includeRuntime")); d != nil {
		diags = append(diags, d...)
	} else if node.GetMap("includeRuntime") != nil {
		pkg.IncludeRuntime = v
	}
	if v, d := optionalBool(file, "/spec/package/embedImages", node.GetMap("embedImages")); d != nil {
		diags = append(diags, d...)
	} else if node.GetMap("embedImages") != nil {
		pkg.EmbedImages = v
	}
	return pkg, diags
}

func optionalBool(file, ptr string, node *loader.Node) (bool, []diagnostic.Diagnostic) {
	if node == nil {
		return false, nil
	}
	if node.Kind != loader.KindBool {
		return false, []diagnostic.Diagnostic{typeErr(file, ptr, "boolean", node)}
	}
	return node.Bool, nil
}

func decodeXUI(file string, node *loader.Node) (*domain.XUI, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/x-ui", "object", node)}
	}
	diags = append(diags, rejectUnknown(file, "/spec/x-ui", node, map[string]struct{}{
		"nodes": {}, "viewport": {},
	})...)
	xui := &domain.XUI{}
	if nodes := node.GetMap("nodes"); nodes != nil {
		if nodes.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, "/spec/x-ui/nodes", "object", nodes))
		} else {
			xui.Nodes = map[string]domain.XUINode{}
			for _, e := range nodes.Map {
				ptr := "/spec/x-ui/nodes/" + escape(e.Key)
				if !identifierRe.MatchString(e.Key) {
					diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
						fmt.Sprintf("x-ui node id %q is not a valid identifier", e.Key)).
						WithSource(file, ptr, e.KeyLine, e.KeyColumn))
				}
				if e.Value == nil || e.Value.Kind != loader.KindMap {
					diags = append(diags, typeErr(file, ptr, "object", e.Value))
					continue
				}
				diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{
					"x": {}, "y": {}, "category": {},
				})...)
				xn := domain.XUINode{}
				x, d := requireNumber(file, ptr+"/x", e.Value.GetMap("x"))
				diags = append(diags, d...)
				y, d := requireNumber(file, ptr+"/y", e.Value.GetMap("y"))
				diags = append(diags, d...)
				if !isFinite(x) || !isFinite(y) {
					diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
						"x-ui node coordinates must be finite"), file, ptr, e.Value))
				}
				xn.X, xn.Y = x, y
				if cat := e.Value.GetMap("category"); cat != nil {
					if cat.Kind != loader.KindString {
						diags = append(diags, typeErr(file, ptr+"/category", "string", cat))
					} else {
						switch cat.Str {
						case "app", "database", "proxy", "cache", "custom":
							xn.Category = cat.Str
						default:
							diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
								fmt.Sprintf("unsupported x-ui category %q", cat.Str)), file, ptr+"/category", cat))
						}
					}
				}
				xui.Nodes[e.Key] = xn
			}
		}
	}
	if vp := node.GetMap("viewport"); vp != nil {
		if vp.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, "/spec/x-ui/viewport", "object", vp))
		} else {
			diags = append(diags, rejectUnknown(file, "/spec/x-ui/viewport", vp, map[string]struct{}{
				"x": {}, "y": {}, "zoom": {},
			})...)
			x, d := requireNumber(file, "/spec/x-ui/viewport/x", vp.GetMap("x"))
			diags = append(diags, d...)
			y, d := requireNumber(file, "/spec/x-ui/viewport/y", vp.GetMap("y"))
			diags = append(diags, d...)
			z, d := requireNumber(file, "/spec/x-ui/viewport/zoom", vp.GetMap("zoom"))
			diags = append(diags, d...)
			if !isFinite(x) || !isFinite(y) || !isFinite(z) || z < 0.1 || z > 4 {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"x-ui viewport requires finite x,y and zoom in 0.1..4"), file, "/spec/x-ui/viewport", vp))
			}
			xui.Viewport = &domain.XUIViewport{X: x, Y: y, Zoom: z}
		}
	}
	return xui, diags
}

func decodeEnvMap(file, ptr string, node *loader.Node) (map[string]domain.ValueSource, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	out := make(map[string]domain.ValueSource, len(node.Map))
	for _, e := range node.Map {
		if e.Key == "" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidValue, "environment key must be non-empty").
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		vs, d := decodeValueSource(file, ptr+"/"+escape(e.Key), e.Value)
		diags = append(diags, d...)
		out[e.Key] = vs
	}
	return out, diags
}

func decodeImageRefParameter(file, ptr string, node *loader.Node) (string, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node == nil || node.Kind != loader.KindMap {
		return "", []diagnostic.Diagnostic{typeErr(file, ptr, "string or {parameter}", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{"parameter": {}})...)
	par := node.GetMap("parameter")
	if par == nil {
		diags = append(diags, missing(file, ptr+"/parameter", node))
		return "", diags
	}
	if par.Kind != loader.KindString || !identifierRe.MatchString(par.Str) {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
			"parameter reference must be a valid identifier"), file, ptr+"/parameter", par))
		return "", diags
	}
	return par.Str, diags
}

func decodeValueSource(file, ptr string, node *loader.Node) (domain.ValueSource, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var vs domain.ValueSource
	if node == nil || node.Kind != loader.KindMap {
		return vs, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	// Reject ambient interpolation shapes. secret is a reference id, never a value.
	for _, e := range node.Map {
		if e.Key == "env" || e.Key == "environment" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestSecretMisuse,
				fmt.Sprintf("value source must not use %q", e.Key)).
				WithSource(file, ptr+"/"+escape(e.Key), e.KeyLine, e.KeyColumn))
		}
	}
	lit := node.GetMap("literal")
	par := node.GetMap("parameter")
	sec := node.GetMap("secret")
	parts := node.GetMap("parts")
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"literal": {}, "parameter": {}, "secret": {}, "parts": {},
	})...)
	nset := 0
	if lit != nil {
		nset++
	}
	if par != nil {
		nset++
	}
	if sec != nil {
		nset++
	}
	if parts != nil {
		nset++
	}
	if nset != 1 {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"value source must be exactly one of {literal}, {parameter}, {secret}, or {parts}"), file, ptr, node))
		return vs, diags
	}
	if parts != nil {
		if parts.Kind != loader.KindSeq || len(parts.Seq) == 0 {
			diags = append(diags, typeErr(file, ptr+"/parts", "non-empty array", parts))
			return vs, diags
		}
		for i, p := range parts.Seq {
			pp := fmt.Sprintf("%s/parts/%d", ptr, i)
			part, d := decodeValueSource(file, pp, p)
			diags = append(diags, d...)
			if len(part.Parts) > 0 {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"nested value source parts are not allowed"), file, pp, p))
				continue
			}
			vs.Parts = append(vs.Parts, part)
		}
		return vs, diags
	}
	if sec != nil {
		if sec.Kind != loader.KindString || !identifierRe.MatchString(sec.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				"secret reference must be a valid identifier"), file, ptr+"/secret", sec))
			return vs, diags
		}
		vs.Secret = sec.Str
		return vs, diags
	}
	if lit != nil {
		sc, d := decodeScalar(file, ptr+"/literal", lit)
		diags = append(diags, d...)
		if sc.Kind == domain.ScalarString && looksLikeInterpolation(sc.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestInterpolationForbidden,
				"${VAR} interpolation is not supported"), file, ptr+"/literal", lit).
				WithHint("Use {literal: ...} or {parameter: id}"))
		}
		vs.Literal = &sc
		return vs, diags
	}
	if par.Kind != loader.KindString || !identifierRe.MatchString(par.Str) {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
			"parameter reference must be a valid identifier"), file, ptr+"/parameter", par))
	} else {
		vs.Parameter = par.Str
	}
	return vs, diags
}

func looksLikeInterpolation(s string) bool {
	return strings.Contains(s, "${")
}

func decodeSecretMounts(file, ptr string, node *loader.Node) ([]domain.SecretMount, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	var out []domain.SecretMount
	for i, item := range node.Seq {
		ip := fmt.Sprintf("%s/%d", ptr, i)
		if item == nil || item.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ip, "object", item))
			continue
		}
		diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{"ref": {}, "target": {}})...)
		ref, d := requireString(file, item, "ref", ip+"/ref")
		diags = append(diags, d...)
		tgt, d := requireString(file, item, "target", ip+"/target")
		diags = append(diags, d...)
		sm := domain.SecretMount{Ref: ref}
		if tgt != "" {
			cp, err := path.ParseContainerPath(tgt)
			if err != nil {
				n := item.GetMap("target")
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
					file, ip+"/target", n))
			} else {
				sm.Target = cp
			}
		}
		out = append(out, sm)
	}
	return out, diags
}

func decodePorts(file, ptr string, node *loader.Node) ([]domain.Port, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	var out []domain.Port
	for i, item := range node.Seq {
		ip := fmt.Sprintf("%s/%d", ptr, i)
		if item == nil || item.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ip, "object", item))
			continue
		}
		diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{
			"container": {}, "host": {}, "bind": {}, "protocol": {},
		})...)
		var p domain.Port
		cn := item.GetMap("container")
		if cn == nil {
			diags = append(diags, missing(file, ip+"/container", item))
		} else {
			n, d := requireInt(file, ip+"/container", cn)
			diags = append(diags, d...)
			if n < 1 || n > 65535 {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"container port must be 1..65535"), file, ip+"/container", cn))
			}
			p.Container = n
		}
		if host := item.GetMap("host"); host != nil {
			hp, d := decodeHostPort(file, ip+"/host", host)
			diags = append(diags, d...)
			p.Host = &hp
		}
		p.Protocol = "tcp"
		if proto := item.GetMap("protocol"); proto != nil {
			if proto.Kind != loader.KindString || (proto.Str != "tcp" && proto.Str != "udp") {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					`protocol must be "tcp" or "udp"`), file, ip+"/protocol", proto))
			} else {
				p.Protocol = proto.Str
			}
		}
		if bind := item.GetMap("bind"); bind != nil {
			if bind.Kind != loader.KindString || bind.Str == "" {
				diags = append(diags, typeErr(file, ip+"/bind", "non-empty string", bind))
			} else if net.ParseIP(bind.Str) == nil {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"bind must be an IP literal (hostnames are forbidden)"), file, ip+"/bind", bind))
			} else {
				p.Bind = bind.Str
			}
		}
		out = append(out, p)
	}
	return out, diags
}

func decodeHostPort(file, ptr string, node *loader.Node) (domain.HostPort, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var hp domain.HostPort
	if node.Kind == loader.KindNumber || node.Kind == loader.KindString {
		n, d := requireInt(file, ptr, node)
		diags = append(diags, d...)
		if n < 1 || n > 65535 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"host port must be 1..65535"), file, ptr, node))
		}
		hp.Literal = &n
		return hp, diags
	}
	if node.Kind != loader.KindMap {
		return hp, []diagnostic.Diagnostic{typeErr(file, ptr, "integer or value source object", node)}
	}
	vs, d := decodeValueSource(file, ptr, node)
	diags = append(diags, d...)
	if vs.Parameter != "" {
		hp.Parameter = vs.Parameter
	} else if vs.Literal != nil {
		switch vs.Literal.Kind {
		case domain.ScalarNumber:
			n := int(vs.Literal.Num)
			if float64(n) != vs.Literal.Num || n < 1 || n > 65535 {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"host port literal must be an integer 1..65535"), file, ptr+"/literal", node.GetMap("literal")))
			} else {
				hp.Literal = &n
			}
		default:
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"host port literal must be an integer"), file, ptr+"/literal", node.GetMap("literal")))
		}
	}
	return hp, diags
}

func decodeMounts(file, ptr string, node *loader.Node) ([]domain.Mount, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	var out []domain.Mount
	for i, item := range node.Seq {
		ip := fmt.Sprintf("%s/%d", ptr, i)
		if item == nil || item.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ip, "object", item))
			continue
		}
		hasVol := item.GetMap("volume") != nil
		hasRes := item.GetMap("resource") != nil
		hasHost := item.GetMap("host") != nil
		hasBuild := item.GetMap("build") != nil
		nkind := 0
		if hasVol {
			nkind++
		}
		if hasRes {
			nkind++
		}
		if hasHost {
			nkind++
		}
		if hasBuild {
			nkind++
		}
		if nkind != 1 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"mount must set exactly one of volume, resource, host, or build"), file, ip, item))
			continue
		}
		var m domain.Mount
		if hasHost {
			diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{
				"host": {}, "target": {}, "readOnly": {},
			})...)
			host := item.GetMap("host")
			switch host.Kind {
			case loader.KindString:
				if host.Str == "" {
					diags = append(diags, typeErr(file, ip+"/host", "non-empty string or {parameter}", host))
				} else {
					m.HostLiteral = host.Str
				}
			case loader.KindMap:
				diags = append(diags, rejectUnknown(file, ip+"/host", host, map[string]struct{}{"parameter": {}})...)
				pid, d := requireString(file, host, "parameter", ip+"/host/parameter")
				diags = append(diags, d...)
				if pid != "" && !identifierRe.MatchString(pid) {
					diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
						"host parameter must be a valid identifier"), file, ip+"/host/parameter", host.GetMap("parameter")))
				}
				m.HostParameter = pid
			default:
				diags = append(diags, typeErr(file, ip+"/host", "string or {parameter}", host))
			}
			m.ReadOnly = false
		} else if hasBuild {
			diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{
				"build": {}, "target": {}, "readOnly": {},
			})...)
			bid, d := requireString(file, item, "build", ip+"/build")
			diags = append(diags, d...)
			if bid != "" && !identifierRe.MatchString(bid) {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
					"build reference must be a valid identifier"), file, ip+"/build", item.GetMap("build")))
			}
			m.Build = bid
			m.ReadOnly = true
		} else if hasVol {
			diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{
				"volume": {}, "target": {}, "readOnly": {},
			})...)
			vol, d := requireString(file, item, "volume", ip+"/volume")
			diags = append(diags, d...)
			m.Volume = vol
			m.ReadOnly = false
		} else {
			diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{
				"resource": {}, "source": {}, "target": {}, "readOnly": {},
			})...)
			res, d := requireString(file, item, "resource", ip+"/resource")
			diags = append(diags, d...)
			m.Resource = res
			m.ReadOnly = true
			if src := item.GetMap("source"); src != nil {
				if src.Kind != loader.KindString || src.Str == "" {
					diags = append(diags, typeErr(file, ip+"/source", "non-empty string", src))
				} else {
					m.Source = src.Str
				}
			}
		}
		tgt, d := requireString(file, item, "target", ip+"/target")
		diags = append(diags, d...)
		if tgt != "" {
			cp, err := path.ParseContainerPath(tgt)
			if err != nil {
				n := item.GetMap("target")
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
					file, ip+"/target", n))
			} else {
				m.Target = cp
			}
		}
		if ro := item.GetMap("readOnly"); ro != nil {
			if ro.Kind != loader.KindBool {
				diags = append(diags, typeErr(file, ip+"/readOnly", "boolean", ro))
			} else {
				m.ReadOnly = ro.Bool
			}
		}
		out = append(out, m)
	}
	return out, diags
}

func decodeDependencies(file, ptr string, node *loader.Node) ([]domain.Dependency, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	var out []domain.Dependency
	for i, item := range node.Seq {
		ip := fmt.Sprintf("%s/%d", ptr, i)
		if item == nil || item.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ip, "object", item))
			continue
		}
		diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{"target": {}, "condition": {}})...)
		tgt, d := requireString(file, item, "target", ip+"/target")
		diags = append(diags, d...)
		cond, d := requireString(file, item, "condition", ip+"/condition")
		diags = append(diags, d...)
		switch cond {
		case "started", "healthy", "completedSuccessfully", "":
		default:
			n := item.GetMap("condition")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				fmt.Sprintf("unsupported dependency condition %q", cond)), file, ip+"/condition", n))
		}
		out = append(out, domain.Dependency{Target: tgt, Condition: cond})
	}
	return out, diags
}

func decodeHealthcheck(file, ptr string, node *loader.Node) (domain.Healthcheck, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var h domain.Healthcheck
	if node.Kind != loader.KindMap {
		return h, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"command": {}, "interval": {}, "timeout": {}, "startPeriod": {}, "retries": {},
	})...)
	cmd := node.GetMap("command")
	if cmd == nil {
		diags = append(diags, missing(file, ptr+"/command", node))
	} else {
		argv, d := decodeArgv(file, ptr+"/command", cmd)
		diags = append(diags, d...)
		h.Command = argv
	}
	if iv := node.GetMap("interval"); iv == nil {
		diags = append(diags, missing(file, ptr+"/interval", node))
	} else {
		dur, d := decodeDuration(file, ptr+"/interval", iv)
		diags = append(diags, d...)
		h.Interval = dur
	}
	if to := node.GetMap("timeout"); to == nil {
		diags = append(diags, missing(file, ptr+"/timeout", node))
	} else {
		dur, d := decodeDuration(file, ptr+"/timeout", to)
		diags = append(diags, d...)
		h.Timeout = dur
	}
	if sp := node.GetMap("startPeriod"); sp != nil {
		dur, d := decodeDuration(file, ptr+"/startPeriod", sp)
		diags = append(diags, d...)
		h.StartPeriod = dur
	}
	if rt := node.GetMap("retries"); rt == nil {
		diags = append(diags, missing(file, ptr+"/retries", node))
	} else {
		n, d := requireInt(file, ptr+"/retries", rt)
		diags = append(diags, d...)
		if n < 1 || n > 100 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"healthcheck.retries must be 1..100"), file, ptr+"/retries", rt))
		}
		h.Retries = n
	}
	return h, diags
}

func decodeResourceLimits(file, ptr string, node *loader.Node) (domain.ResourceLimits, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var rl domain.ResourceLimits
	if node.Kind != loader.KindMap {
		return rl, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{"cpus": {}, "memory": {}, "gpu": {}, "ldLibraryPath": {}})...)
	if cpus := node.GetMap("cpus"); cpus != nil {
		if cpus.Kind != loader.KindString || !cpusRe.MatchString(cpus.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"resources.cpus must be a decimal string in 0.001..1024"), file, ptr+"/cpus", cpus))
		} else {
			rl.CPUs = cpus.Str
		}
	}
	if mem := node.GetMap("memory"); mem != nil {
		n, d := requireInt64(file, ptr+"/memory", mem)
		diags = append(diags, d...)
		if n < minMemoryBytes || n > maxMemoryBytes {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"resources.memory must be 1 MiB .. 1 TiB"), file, ptr+"/memory", mem))
		}
		rl.Memory = n
	}
	if gpu := node.GetMap("gpu"); gpu != nil {
		switch {
		case gpu.Kind == loader.KindString && strings.TrimSpace(gpu.Str) == "all":
			rl.GPUAll = true
		case gpu.Kind == loader.KindString && strings.TrimSpace(gpu.Str) == "devices-only":
			rl.GPUDevicesOnly = true
		default:
			n, d := requireInt(file, ptr+"/gpu", gpu)
			diags = append(diags, d...)
			if n < 1 || n > 8 {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"resources.gpu must be 1..8, all, or devices-only"), file, ptr+"/gpu", gpu))
			} else {
				rl.GPU = n
			}
		}
	}
	if lib := node.GetMap("ldLibraryPath"); lib != nil {
		if lib.Kind != loader.KindString || !validLDLibraryPath(lib.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath,
				"ldLibraryPath must be an absolute container path without .."), file, ptr+"/ldLibraryPath", lib))
		} else {
			rl.LDLibraryPath = lib.Str
		}
	}
	if rl.GPUDevicesOnly && rl.LDLibraryPath == "" {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaMissingField,
			"gpu: devices-only requires ldLibraryPath"), file, ptr+"/ldLibraryPath", node))
	}
	if rl.LDLibraryPath != "" && !rl.GPUDevicesOnly {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"ldLibraryPath is only valid with gpu: devices-only"), file, ptr+"/ldLibraryPath", node.GetMap("ldLibraryPath")))
	}
	return rl, diags
}

func validLDLibraryPath(s string) bool {
	if s == "" || !strings.HasPrefix(s, "/") || strings.Contains(s, "..") || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	return true
}

// NormalizeResourceCPUs maps Compose-style CPU limits (e.g. "2.0") to a manifest-valid decimal string.
func NormalizeResourceCPUs(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if cpusRe.MatchString(s) {
		return s, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0.001 || f > 1024 {
		return "", false
	}
	var normalized string
	if f == float64(int64(f)) && f >= 1 {
		normalized = strconv.FormatInt(int64(f), 10)
	} else {
		normalized = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if !cpusRe.MatchString(normalized) {
		return "", false
	}
	return normalized, true
}

func decodeRetry(file, ptr string, node *loader.Node) (domain.Retry, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var r domain.Retry
	if node.Kind != loader.KindMap {
		return r, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"count": {}, "initialBackoff": {}, "multiplier": {}, "maxBackoff": {},
	})...)
	if c := node.GetMap("count"); c != nil {
		n, d := requireInt(file, ptr+"/count", c)
		diags = append(diags, d...)
		if n < 0 || n > 10 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"retry.count must be 0..10"), file, ptr+"/count", c))
		}
		r.Count = n
	}
	if ib := node.GetMap("initialBackoff"); ib != nil {
		dur, d := decodeDuration(file, ptr+"/initialBackoff", ib)
		diags = append(diags, d...)
		if ms, ok := durationMillis(dur); ok && (ms < 100 || ms > 5*60*1000) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidDuration,
				"initialBackoff must be between 100ms and 5m"), file, ptr+"/initialBackoff", ib))
		}
		r.InitialBackoff = dur
	}
	if m := node.GetMap("multiplier"); m != nil {
		f, d := requireNumber(file, ptr+"/multiplier", m)
		diags = append(diags, d...)
		if f < 1 || f > 10 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"retry.multiplier must be 1..10"), file, ptr+"/multiplier", m))
		}
		r.Multiplier = f
	}
	if mb := node.GetMap("maxBackoff"); mb != nil {
		dur, d := decodeDuration(file, ptr+"/maxBackoff", mb)
		diags = append(diags, d...)
		if ms, ok := durationMillis(dur); ok && ms > 5*60*1000 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidDuration,
				"maxBackoff must be <= 5m"), file, ptr+"/maxBackoff", mb))
		}
		r.MaxBackoff = dur
	}
	return r, diags
}

func decodeCommand(file, ptr string, node *loader.Node) ([]domain.ArgvElement, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node == nil || node.Kind != loader.KindSeq || len(node.Seq) == 0 {
		return nil, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"command must be a non-empty argv array"), file, ptr, node)}
	}
	out := make([]domain.ArgvElement, 0, len(node.Seq))
	for i, item := range node.Seq {
		ip := fmt.Sprintf("%s/%d", ptr, i)
		if item == nil {
			diags = append(diags, typeErr(file, ip, "string or {parts}", item))
			continue
		}
		switch item.Kind {
		case loader.KindString:
			if item.Str == "" {
				diags = append(diags, typeErr(file, ip, "non-empty string", item))
				continue
			}
			out = append(out, domain.ArgvElement{Literal: item.Str})
		case loader.KindMap:
			if lit := item.GetMap("literal"); lit != nil && item.GetMap("parts") == nil && item.GetMap("secret") == nil {
				diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{"literal": {}})...)
				if lit.Kind != loader.KindString {
					diags = append(diags, typeErr(file, ip+"/literal", "string", lit))
					continue
				}
				out = append(out, domain.ArgvElement{Literal: lit.Str})
				continue
			}
			if sec := item.GetMap("secret"); sec != nil && item.GetMap("parts") == nil {
				diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{"secret": {}})...)
				if sec.Kind != loader.KindString || !identifierRe.MatchString(sec.Str) {
					diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
						"argv secret must be a valid identifier"), file, ip+"/secret", sec))
					continue
				}
				out = append(out, domain.ArgvElement{Secret: sec.Str})
				continue
			}
			parts := item.GetMap("parts")
			diags = append(diags, rejectUnknown(file, ip, item, map[string]struct{}{"parts": {}})...)
			if parts == nil || parts.Kind != loader.KindSeq || len(parts.Seq) == 0 {
				diags = append(diags, typeErr(file, ip+"/parts", "non-empty array", parts))
				continue
			}
			el := domain.ArgvElement{}
			for j, p := range parts.Seq {
				pp := fmt.Sprintf("%s/parts/%d", ip, j)
				if p == nil {
					diags = append(diags, typeErr(file, pp, "string or {parameter}", p))
					continue
				}
				switch p.Kind {
				case loader.KindString:
					if p.Str == "" {
						diags = append(diags, typeErr(file, pp, "non-empty string", p))
						continue
					}
					el.Parts = append(el.Parts, domain.ArgvPart{Literal: p.Str})
				case loader.KindMap:
					if sec := p.GetMap("secret"); sec != nil && p.GetMap("parameter") == nil {
						diags = append(diags, rejectUnknown(file, pp, p, map[string]struct{}{"secret": {}})...)
						if sec.Kind != loader.KindString || !identifierRe.MatchString(sec.Str) {
							diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
								"argv secret must be a valid identifier"), file, pp+"/secret", sec))
							continue
						}
						el.Parts = append(el.Parts, domain.ArgvPart{Secret: sec.Str})
						continue
					}
					diags = append(diags, rejectUnknown(file, pp, p, map[string]struct{}{"parameter": {}})...)
					pid, d := requireString(file, p, "parameter", pp+"/parameter")
					diags = append(diags, d...)
					if pid != "" && !identifierRe.MatchString(pid) {
						diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
							"argv parameter must be a valid identifier"), file, pp+"/parameter", p.GetMap("parameter")))
					}
					el.Parts = append(el.Parts, domain.ArgvPart{Parameter: pid})
				default:
					diags = append(diags, typeErr(file, pp, "string or {parameter}", p))
				}
			}
			out = append(out, el)
		default:
			diags = append(diags, typeErr(file, ip, "string or {parts}", item))
		}
	}
	return out, diags
}

func decodeArgv(file, ptr string, node *loader.Node) ([]string, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	if len(node.Seq) == 0 {
		return nil, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"argv array must be non-empty"), file, ptr, node)}
	}
	out := make([]string, 0, len(node.Seq))
	for i, item := range node.Seq {
		if item == nil || item.Kind != loader.KindString || item.Str == "" {
			diags = append(diags, typeErr(file, fmt.Sprintf("%s/%d", ptr, i), "non-empty string", item))
			continue
		}
		// Reject shell-form markers.
		if strings.EqualFold(item.Str, "CMD-SHELL") {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"shell/CMD-SHELL healthcheck forms are forbidden"), file, fmt.Sprintf("%s/%d", ptr, i), item))
		}
		out = append(out, item.Str)
	}
	return out, diags
}

func decodeIDList(file, ptr string, node *loader.Node) ([]string, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	out := make([]string, 0, len(node.Seq))
	for i, item := range node.Seq {
		if item == nil || item.Kind != loader.KindString || !identifierRe.MatchString(item.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				"list entry must be a valid identifier"), file, fmt.Sprintf("%s/%d", ptr, i), item))
			continue
		}
		out = append(out, item.Str)
	}
	return out, diags
}

func decodeProfileNames(file, ptr string, node *loader.Node) ([]string, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindSeq {
		return nil, []diagnostic.Diagnostic{typeErr(file, ptr, "array", node)}
	}
	out := make([]string, 0, len(node.Seq))
	seen := map[string]struct{}{}
	for i, item := range node.Seq {
		if item == nil || item.Kind != loader.KindString || !profileNameRe.MatchString(item.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"profile name must match ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$"), file, fmt.Sprintf("%s/%d", ptr, i), item))
			continue
		}
		if _, ok := seen[item.Str]; ok {
			continue
		}
		seen[item.Str] = struct{}{}
		out = append(out, item.Str)
	}
	return out, diags
}

func decodeDuration(file, ptr string, node *loader.Node) (string, []diagnostic.Diagnostic) {
	if node == nil || node.Kind != loader.KindString {
		return "", []diagnostic.Diagnostic{typeErr(file, ptr, "string", node)}
	}
	if !durationRe.MatchString(node.Str) {
		return "", []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidDuration,
			"duration must match <positive-int>(ms|s|m|h)"), file, ptr, node)}
	}
	ms, ok := durationMillis(node.Str)
	if !ok || ms <= 0 || ms > maxDuration {
		return "", []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidDuration,
			"duration must be > 0 and <= 24h"), file, ptr, node)}
	}
	return node.Str, nil
}

func durationMillis(s string) (int64, bool) {
	m := durationRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	switch m[2] {
	case "ms":
		return n, true
	case "s":
		return n * 1000, true
	case "m":
		return n * 60 * 1000, true
	case "h":
		return n * 60 * 60 * 1000, true
	default:
		return 0, false
	}
}

func decodeScalar(file, ptr string, node *loader.Node) (domain.Scalar, []diagnostic.Diagnostic) {
	var sc domain.Scalar
	if node == nil {
		return sc, []diagnostic.Diagnostic{typeErr(file, ptr, "scalar", node)}
	}
	switch node.Kind {
	case loader.KindString:
		sc.Kind = domain.ScalarString
		sc.Str = node.Str
	case loader.KindNumber:
		sc.Kind = domain.ScalarNumber
		f, err := strconv.ParseFloat(node.Str, 64)
		if err != nil {
			return sc, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"invalid number"), file, ptr, node)}
		}
		sc.Num = f
		sc.Str = node.Str
	case loader.KindBool:
		sc.Kind = domain.ScalarBool
		sc.Bool = node.Bool
	default:
		return sc, []diagnostic.Diagnostic{typeErr(file, ptr, "string|number|boolean", node)}
	}
	return sc, nil
}

func scalarKey(sc domain.Scalar) string {
	switch sc.Kind {
	case domain.ScalarString:
		return "s:" + sc.Str
	case domain.ScalarNumber:
		return "n:" + sc.Str
	case domain.ScalarBool:
		if sc.Bool {
			return "b:true"
		}
		return "b:false"
	default:
		return "?"
	}
}

func requireNumber(file, ptr string, node *loader.Node) (float64, []diagnostic.Diagnostic) {
	if node == nil {
		return 0, []diagnostic.Diagnostic{missing(file, ptr, nil)}
	}
	if node.Kind == loader.KindNumber {
		f, err := strconv.ParseFloat(node.Str, 64)
		if err != nil {
			return 0, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"invalid number"), file, ptr, node)}
		}
		return f, nil
	}
	return 0, []diagnostic.Diagnostic{typeErr(file, ptr, "number", node)}
}

func requireInt(file, ptr string, node *loader.Node) (int, []diagnostic.Diagnostic) {
	n64, d := requireInt64(file, ptr, node)
	return int(n64), d
}

func requireInt64(file, ptr string, node *loader.Node) (int64, []diagnostic.Diagnostic) {
	if node == nil {
		return 0, []diagnostic.Diagnostic{missing(file, ptr, nil)}
	}
	switch node.Kind {
	case loader.KindNumber:
		if strings.ContainsAny(node.Str, ".eE") {
			return 0, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"expected integer"), file, ptr, node)}
		}
		n, err := strconv.ParseInt(node.Str, 10, 64)
		if err != nil {
			return 0, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"invalid integer"), file, ptr, node)}
		}
		return n, nil
	case loader.KindString:
		n, err := strconv.ParseInt(node.Str, 10, 64)
		if err != nil {
			return 0, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"invalid integer"), file, ptr, node)}
		}
		return n, nil
	default:
		return 0, []diagnostic.Diagnostic{typeErr(file, ptr, "integer", node)}
	}
}
