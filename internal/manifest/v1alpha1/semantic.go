package v1alpha1

import (
	"fmt"
	"net"
	"sort"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
)

type vsRef struct{ param, secret string }

func valueSourceRefs(vs domain.ValueSource) []vsRef {
	if len(vs.Parts) > 0 {
		var out []vsRef
		for _, p := range vs.Parts {
			out = append(out, valueSourceRefs(p)...)
		}
		return out
	}
	if vs.Parameter == "" && vs.Secret == "" {
		return nil
	}
	return []vsRef{{param: vs.Parameter, secret: vs.Secret}}
}

func semanticValidate(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	diags = append(diags, validateCrossRefs(file, p)...)
	diags = append(diags, validateEnvironmentFrom(file, p)...)
	diags = append(diags, validateDatabases(file, p)...)
	diags = append(diags, validatePortConflicts(file, p)...)
	diags = append(diags, validateDependencyRules(file, p)...)
	diags = append(diags, validateXUIRefs(file, p)...)
	return diags
}

func validateEnvironmentFrom(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	ids := make([]string, 0, len(p.Spec.Services))
	for id := range p.Spec.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		svc := p.Spec.Services[id]
		if svc.EnvironmentFrom == "" {
			continue
		}
		ptr := "/spec/services/" + escape(id) + "/environmentFrom"
		if svc.EnvironmentFrom == id {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("service %q environmentFrom references itself", id)).
				WithSource(file, ptr, 0, 0))
			continue
		}
		if _, ok := p.Spec.Services[svc.EnvironmentFrom]; !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("service %q environmentFrom references unknown service %q", id, svc.EnvironmentFrom)).
				WithSource(file, ptr, 0, 0))
			continue
		}
		seen := map[string]bool{id: true}
		cur := svc.EnvironmentFrom
		for cur != "" {
			if seen[cur] {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("service %q environmentFrom cycle through %q", id, cur)).
					WithSource(file, ptr, 0, 0))
				break
			}
			seen[cur] = true
			next, ok := p.Spec.Services[cur]
			if !ok {
				break
			}
			cur = next.EnvironmentFrom
		}
	}
	return diags
}

func validateDatabases(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	ids := make([]string, 0, len(p.Spec.Databases))
	for id := range p.Spec.Databases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		db := p.Spec.Databases[id]
		ptr := "/spec/databases/" + escape(id)
		if _, ok := p.Spec.Services[db.On]; db.On != "" && !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q on unknown service %q", id, db.On)).
				WithSource(file, ptr+"/on", 0, 0))
		}
		if _, ok := p.Spec.Services[db.OwnerFrom]; db.OwnerFrom != "" && !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q ownerFrom unknown service %q", id, db.OwnerFrom)).
				WithSource(file, ptr+"/ownerFrom", 0, 0))
		}
		taskID := "db-" + id
		if _, ok := p.Spec.Tasks[taskID]; ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q task id %q already exists", id, taskID)).
				WithSource(file, ptr, 0, 0))
		}
		owner := p.Spec.Services[db.OwnerFrom]
		if db.OwnerFrom == "" {
			continue
		}
		if onSvc, ok := p.Spec.Services[db.On]; ok && onSvc.Healthcheck == nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q service %q needs a healthcheck so the migration waits for healthy", id, db.On)).
				WithSource(file, ptr+"/on", 0, 0))
		}
		if _, ok := owner.Environment["POSTGRES_USER"]; !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q owner service %q has no POSTGRES_USER", id, db.OwnerFrom)).
				WithSource(file, ptr+"/ownerFrom", 0, 0))
		}
		if ownerPasswordSecret(owner) == "" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("database %q owner service %q has no password secret (POSTGRES_PASSWORD or secret mount)", id, db.OwnerFrom)).
				WithSource(file, ptr+"/ownerFrom", 0, 0))
		}
	}
	return diags
}

func ownerPasswordSecret(svc domain.Service) string {
	if vs, ok := svc.Environment["POSTGRES_PASSWORD"]; ok && vs.Secret != "" {
		return vs.Secret
	}
	for _, sm := range svc.Secrets {
		if sm.Ref != "" {
			return sm.Ref
		}
	}
	return ""
}

func validateCrossRefs(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	for id, svc := range p.Spec.Services {
		ptr := "/spec/services/" + escape(id)
		if _, ok := p.Spec.Images[svc.Image]; svc.Image != "" && !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("service %q references unknown image %q", id, svc.Image)).
				WithSource(file, ptr+"/image", 0, 0))
		}
		for k, vs := range svc.Environment {
			for _, ref := range valueSourceRefs(vs) {
				if ref.param != "" {
					if _, ok := p.Spec.Parameters[ref.param]; !ok {
						diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
							fmt.Sprintf("service %q environment %q references unknown parameter %q", id, k, ref.param)).
							WithSource(file, ptr+"/environment/"+escape(k), 0, 0))
					}
				}
				if ref.secret != "" {
					if _, ok := p.Spec.Secrets[ref.secret]; !ok {
						diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
							fmt.Sprintf("service %q environment %q references unknown secret %q", id, k, ref.secret)).
							WithSource(file, ptr+"/environment/"+escape(k), 0, 0))
					}
				}
			}
		}
		for i, sm := range svc.Secrets {
			if _, ok := p.Spec.Secrets[sm.Ref]; !ok {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("service %q secret mount references unknown secret %q", id, sm.Ref)).
					WithSource(file, fmt.Sprintf("%s/secrets/%d/ref", ptr, i), 0, 0))
			}
		}
		for i, el := range svc.Command {
			if el.Secret != "" {
				if _, ok := p.Spec.Secrets[el.Secret]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q command references unknown secret %q", id, el.Secret)).
						WithSource(file, fmt.Sprintf("%s/command/%d", ptr, i), 0, 0))
				}
			}
			for _, part := range el.Parts {
				if part.Parameter != "" {
					if _, ok := p.Spec.Parameters[part.Parameter]; !ok {
						diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
							fmt.Sprintf("service %q command references unknown parameter %q", id, part.Parameter)).
							WithSource(file, fmt.Sprintf("%s/command/%d", ptr, i), 0, 0))
					}
				}
				if part.Secret != "" {
					if _, ok := p.Spec.Secrets[part.Secret]; !ok {
						diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
							fmt.Sprintf("service %q command references unknown secret %q", id, part.Secret)).
							WithSource(file, fmt.Sprintf("%s/command/%d", ptr, i), 0, 0))
					}
				}
			}
		}
		for i, m := range svc.Mounts {
			if m.Build != "" {
				if _, ok := p.Spec.Builds[m.Build]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q mount references unknown build %q", id, m.Build)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/build", ptr, i), 0, 0))
				}
			}
			if m.HostParameter != "" {
				if _, ok := p.Spec.Parameters[m.HostParameter]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q mount references unknown host parameter %q", id, m.HostParameter)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/host", ptr, i), 0, 0))
				}
			}
			if m.Volume != "" {
				if _, ok := p.Spec.Volumes[m.Volume]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q mount references unknown volume %q", id, m.Volume)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/volume", ptr, i), 0, 0))
				}
			}
			if m.Resource != "" {
				if _, ok := p.Spec.Resources[m.Resource]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q mount references unknown resource %q", id, m.Resource)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/resource", ptr, i), 0, 0))
				}
			}
		}
		for i, n := range svc.Networks {
			if _, ok := p.Spec.Networks[n]; !ok {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("service %q references unknown network %q", id, n)).
					WithSource(file, fmt.Sprintf("%s/networks/%d", ptr, i), 0, 0))
			}
		}
		for i, port := range svc.Ports {
			if port.Host != nil && port.Host.Parameter != "" {
				if _, ok := p.Spec.Parameters[port.Host.Parameter]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("service %q port references unknown parameter %q", id, port.Host.Parameter)).
						WithSource(file, fmt.Sprintf("%s/ports/%d/host", ptr, i), 0, 0))
				}
			}
		}
	}
	for id, img := range p.Spec.Images {
		if img.RefParameter != "" {
			if _, ok := p.Spec.Parameters[img.RefParameter]; !ok {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("image %q ref references unknown parameter %q", id, img.RefParameter)).
					WithSource(file, "/spec/images/"+escape(id)+"/ref/parameter", 0, 0))
			}
		}
		if img.Build == nil {
			continue
		}
		for arg, vs := range img.Build.Args {
			if vs.IsParameter() {
				par, ok := p.Spec.Parameters[vs.Parameter]
				if !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("image %q build arg %q references unknown parameter %q", id, arg, vs.Parameter)).
						WithSource(file, "/spec/images/"+escape(id)+"/build/args/"+escape(arg), 0, 0))
				} else if par.Phase != domain.ParameterPhaseBuild {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("image %q build arg %q must reference a build-phase parameter", id, arg)).
						WithSource(file, "/spec/images/"+escape(id)+"/build/args/"+escape(arg), 0, 0))
				}
			}
		}
	}
	for id, task := range p.Spec.Tasks {
		ptr := "/spec/tasks/" + escape(id)
		if _, ok := p.Spec.Images[task.Image]; task.Image != "" && !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("task %q references unknown image %q", id, task.Image)).
				WithSource(file, ptr+"/image", 0, 0))
		}
		for i, sm := range task.Secrets {
			if _, ok := p.Spec.Secrets[sm.Ref]; !ok {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("task %q secret mount references unknown secret %q", id, sm.Ref)).
					WithSource(file, fmt.Sprintf("%s/secrets/%d/ref", ptr, i), 0, 0))
			}
		}
		for i, m := range task.Mounts {
			if m.Volume != "" {
				if _, ok := p.Spec.Volumes[m.Volume]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("task %q mount references unknown volume %q", id, m.Volume)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/volume", ptr, i), 0, 0))
				}
			}
			if m.Resource != "" {
				if _, ok := p.Spec.Resources[m.Resource]; !ok {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
						fmt.Sprintf("task %q mount references unknown resource %q", id, m.Resource)).
						WithSource(file, fmt.Sprintf("%s/mounts/%d/resource", ptr, i), 0, 0))
				}
			}
		}
		for i, n := range task.Networks {
			if _, ok := p.Spec.Networks[n]; !ok {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
					fmt.Sprintf("task %q references unknown network %q", id, n)).
					WithSource(file, fmt.Sprintf("%s/networks/%d", ptr, i), 0, 0))
			}
		}
	}
	return diags
}

type hostBinding struct {
	service  string
	portKey  string // literal or parameter:<id>
	protocol string
	bind     string
	index    int
}

func validatePortConflicts(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	var bindings []hostBinding
	svcIDs := sortedKeys(p.Spec.Services)
	for _, id := range svcIDs {
		svc := p.Spec.Services[id]
		for i, port := range svc.Ports {
			if port.Host == nil {
				continue
			}
			key := ""
			if port.Host.Literal != nil {
				key = fmt.Sprintf("%d", *port.Host.Literal)
			} else if port.Host.Parameter != "" {
				key = "parameter:" + port.Host.Parameter
			} else {
				continue
			}
			bind := port.Bind
			if bind == "" {
				bind = "0.0.0.0"
			}
			bindings = append(bindings, hostBinding{
				service: id, portKey: key, protocol: port.Protocol, bind: bind, index: i,
			})
		}
	}
	for i := 0; i < len(bindings); i++ {
		for j := i + 1; j < len(bindings); j++ {
			a, b := bindings[i], bindings[j]
			if a.portKey != b.portKey || a.protocol != b.protocol {
				continue
			}
			if bindsOverlap(a.bind, b.bind) {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPortConflict,
					fmt.Sprintf("host port conflict between services %q and %q on %s/%s",
						a.service, b.service, a.portKey, a.protocol)).
					WithSource(file, fmt.Sprintf("/spec/services/%s/ports/%d", escape(a.service), a.index), 0, 0).
					WithHint("Use distinct host ports or non-overlapping bind addresses"))
			}
		}
	}
	return diags
}

func bindsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	ai, bi := net.ParseIP(a), net.ParseIP(b)
	if ai == nil || bi == nil {
		return false
	}
	// Treat IPv4-mapped forms as their IPv4 address (::ffff:127.0.0.1 ≡ 127.0.0.1).
	if a4, b4 := ai.To4(), bi.To4(); a4 != nil && b4 != nil && a4.Equal(b4) {
		return true
	}
	if ai.Equal(bi) {
		return true
	}
	a4 := ai.To4() != nil
	b4 := bi.To4() != nil
	// 0.0.0.0 overlaps any IPv4; :: overlaps any IPv4/IPv6 (TZ §9.13).
	if ai.Equal(net.IPv4zero) && b4 {
		return true
	}
	if bi.Equal(net.IPv4zero) && a4 {
		return true
	}
	if ai.Equal(net.IPv6zero) || bi.Equal(net.IPv6zero) {
		return true
	}
	return false
}

func validateDependencyRules(file string, p domain.Project) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	type kindPhase struct {
		kind  string // service|task
		phase domain.TaskPhase
	}
	nodes := map[string]kindPhase{}
	for id := range p.Spec.Services {
		nodes[id] = kindPhase{kind: "service"}
	}
	for id, t := range p.Spec.Tasks {
		if _, exists := nodes[id]; exists {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("task id %q collides with a service id", id)).
				WithSource(file, "/spec/tasks/"+escape(id), 0, 0))
		}
		nodes[id] = kindPhase{kind: "task", phase: t.Phase}
	}

	checkDep := func(dependent string, depKind string, depPhase domain.TaskPhase, dep domain.Dependency, ptr string) {
		target, ok := nodes[dep.Target]
		if !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("%s %q depends on unknown target %q", depKind, dependent, dep.Target)).
				WithSource(file, ptr+"/target", 0, 0))
			return
		}
		if dep.Condition == "healthy" {
			if target.kind != "service" {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
					fmt.Sprintf("%s %q healthy dependency requires a service target", depKind, dependent)).
					WithSource(file, ptr, 0, 0))
			} else if p.Spec.Services[dep.Target].Healthcheck == nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
					fmt.Sprintf("healthy dependency on %q requires a healthcheck", dep.Target)).
					WithSource(file, ptr, 0, 0).
					WithHint("Add healthcheck to the target service or use condition started"))
			}
		}
		if dep.Condition == "completedSuccessfully" && target.kind != "task" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
				fmt.Sprintf("completedSuccessfully requires a task target (dependent %q)", dependent)).
				WithSource(file, ptr, 0, 0))
		}
		// Phase partial order (TZ §9.11).
		if depKind == "task" {
			switch depPhase {
			case domain.TaskPhasePreStart:
				if target.kind == "service" {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("preStart task %q cannot depend on a service", dependent)).
						WithSource(file, ptr, 0, 0))
				} else if target.phase != domain.TaskPhasePreStart {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("preStart task %q may only depend on other preStart tasks", dependent)).
						WithSource(file, ptr, 0, 0))
				}
			case domain.TaskPhaseMigration:
				if target.kind == "task" && target.phase == domain.TaskPhasePostDeploy {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("migration task %q cannot depend on postDeploy", dependent)).
						WithSource(file, ptr, 0, 0))
				}
				if target.kind == "task" && target.phase == domain.TaskPhasePreStart {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("migration task %q cannot depend on preStart task (use services)", dependent)).
						WithSource(file, ptr, 0, 0))
				}
			case domain.TaskPhasePostDeploy:
				// may depend on service, migration, or postDeploy
				if target.kind == "task" && target.phase == domain.TaskPhasePreStart {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("postDeploy task %q cannot depend on preStart task", dependent)).
						WithSource(file, ptr, 0, 0))
				}
			}
			if dep.Condition == "completedSuccessfully" && target.kind == "task" {
				if phaseOrder(target.phase) > phaseOrder(depPhase) {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("task %q cannot require completedSuccessfully of a later-phase task", dependent)).
						WithSource(file, ptr, 0, 0))
				}
			}
		}
		if depKind == "service" {
			if target.kind == "task" {
				switch target.phase {
				case domain.TaskPhasePostDeploy:
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
						fmt.Sprintf("service %q cannot depend on postDeploy task", dependent)).
						WithSource(file, ptr, 0, 0))
				case domain.TaskPhaseMigration:
					if dep.Condition != "completedSuccessfully" {
						diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidDependency,
							fmt.Sprintf("service %q may only depend on migration task with completedSuccessfully", dependent)).
							WithSource(file, ptr, 0, 0).
							WithHint("Use condition: completedSuccessfully so apply starts the service after migrate succeeds"))
					}
				case domain.TaskPhasePreStart:
					// allowed: service ← preStart (started/healthy/completedSuccessfully per condition rules)
				}
			}
		}
	}

	for id, svc := range p.Spec.Services {
		for i, dep := range svc.DependsOn {
			checkDep(id, "service", "", dep, fmt.Sprintf("/spec/services/%s/dependsOn/%d", escape(id), i))
		}
	}
	for id, task := range p.Spec.Tasks {
		for i, dep := range task.DependsOn {
			checkDep(id, "task", task.Phase, dep, fmt.Sprintf("/spec/tasks/%s/dependsOn/%d", escape(id), i))
		}
	}
	return diags
}

func phaseOrder(p domain.TaskPhase) int {
	switch p {
	case domain.TaskPhasePreStart:
		return 0
	case domain.TaskPhaseMigration:
		return 1
	case domain.TaskPhasePostDeploy:
		return 2
	default:
		return -1
	}
}

func validateXUIRefs(file string, p domain.Project) []diagnostic.Diagnostic {
	if p.Spec.XUI == nil {
		return nil
	}
	var diags []diagnostic.Diagnostic
	known := map[string]struct{}{}
	for id := range p.Spec.Services {
		known[id] = struct{}{}
	}
	for id := range p.Spec.Tasks {
		known[id] = struct{}{}
	}
	for id := range p.Spec.XUI.Nodes {
		if _, ok := known[id]; !ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
				fmt.Sprintf("x-ui node %q does not match a service or task id", id)).
				WithSource(file, "/spec/x-ui/nodes/"+escape(id), 0, 0))
		}
	}
	return diags
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SecretCanaryMarker is used in tests; must never appear in diagnostics/inspect.
const SecretCanaryMarker = "SECRET_CANARY_VALUE_DO_NOT_LEAK"
