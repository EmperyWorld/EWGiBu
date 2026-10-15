// Package importpropose builds reviewable Project drafts from Compose/Dockerfiles.
// Propose never writes the workspace (ADR-016).
package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
	"iacpack/internal/inputs"
	"iacpack/internal/manifest/loader"
	v1alpha1 "iacpack/internal/manifest/v1alpha1"
)

// Request is a propose import request.
type Request struct {
	Workspace       string
	Compose         string         // single path; merged into Composes when set
	Composes        []string       // workspace-relative Compose paths (ordered, first-wins)
	Dockerfiles     []string       // workspace-relative Dockerfile paths
	Name            string         // optional metadata.name override
	ActiveProfiles  []string       // Compose profile names (COMPOSE_PROFILES / --profile)
	Strict          bool           // policy denials on included services are errors (CI)
	ExistingProject map[string]any // prior draft; keeps existing service.user
}

// Summary counts draft entities.
type Summary struct {
	Services   int `json:"services"`
	Images     int `json:"images"`
	Networks   int `json:"networks"`
	Volumes    int `json:"volumes"`
	Resources  int `json:"resources"`
	Parameters int `json:"parameters"`
}

// Refusal is a structured import refusal for UI grouping (ADR-017).
type Refusal struct {
	Path     string `json:"path"`
	Pointer  string `json:"pointer,omitempty"`
	Code     string `json:"code"`
	Category string `json:"category"` // policy|unsupported|secret|path|collision
	Message  string `json:"message"`
}

// Result is the propose response (no disk write).
type Result struct {
	OK                bool                    `json:"ok"`
	Project           map[string]any          `json:"project"`
	Summary           Summary                 `json:"summary"`
	Sources           map[string]any          `json:"sources"`
	Refusals          []Refusal               `json:"refusals"`
	ParameterGaps     []ParameterGap          `json:"parameterGaps"`
	SecretSuggestions []SecretSuggestion      `json:"secretSuggestions"`
	EnvExample        string                  `json:"envExample"`
	Diagnostics       []diagnostic.Diagnostic `json:"-"`
}

var (
	idRe     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	interpRe = regexp.MustCompile(`\$\{[^}]+\}|\$[A-Za-z_][A-Za-z0-9_]*`)
	// token matches TOKEN, not TOKENS (a count such as IMAGE_MAX_TOKENS).
	secretNameRe     = regexp.MustCompile(`(?i)(password|secret|token(?:[^s]|$)|apikey|api_key|private.?key|credential)`)
	numericSettingRe = regexp.MustCompile(`(?i)(?:^|_)(ttl_seconds|ttl|seconds|minutes|hours|days|millis|milliseconds|timeout|interval|count|size|port|duration|limit)$`)
)

var deniedServiceKeys = map[string]string{
	"privileged":    diagnostic.CodeImportPolicyDenied,
	"devices":       diagnostic.CodeImportPolicyDenied,
	"cap_add":       diagnostic.CodeImportPolicyDenied,
	"cap_drop":      diagnostic.CodeImportPolicyDenied,
	"network_mode":  diagnostic.CodeImportPolicyDenied,
	"pid":           diagnostic.CodeImportPolicyDenied,
	"ipc":           diagnostic.CodeImportPolicyDenied,
	"userns_mode":   diagnostic.CodeImportPolicyDenied,
	"cgroup_parent": diagnostic.CodeImportPolicyDenied,
	"extends":       diagnostic.CodeImportUnsupportedField,
	"configs":       diagnostic.CodeImportUnsupportedField,
}

var warnOmitServiceKeys = map[string]bool{
	"container_name":    true,
	"logging":           true,
	"hostname":          true,
	"domainname":        true,
	"stdin_open":        true,
	"tty":               true,
	"init":              true,
	"stop_grace_period": true,
	"stop_signal":       true,
	"sysctls":           true,
	"ulimits":           true,
	"extra_hosts":       true,
	"dns":               true,
	"dns_search":        true,
	"tmpfs":             true,
	"shm_size":          true,
	"working_dir":       true, // mapped only when we have Workdir on tasks; services omit for now as warn
	"labels":            true,
	"platform":          true,
	"pull_policy":       true,
	"expose":            true,
	"scale":             true,
	"security_opt":      true,
}

var allowedServiceKeys = map[string]bool{
	"image": true, "build": true, "command": true, "entrypoint": true, "user": true,
	"environment": true, "ports": true, "networks": true, "volumes": true,
	"secrets": true, "depends_on": true, "healthcheck": true, "restart": true,
	"deploy": true,
}

// Propose builds a draft Project from Compose and/or Dockerfiles.
func Propose(req Request) Result {
	var diags []diagnostic.Diagnostic
	var refusals []Refusal
	if strings.TrimSpace(req.Workspace) == "" {
		d := diagnostic.Error(diagnostic.CodeImportPathInvalid, "workspace is required")
		return Result{OK: false, Refusals: refusalsFromDiags([]diagnostic.Diagnostic{d}), Diagnostics: []diagnostic.Diagnostic{d}}
	}

	composePaths := normalizeComposePaths(req)
	dfiles := append([]string(nil), req.Dockerfiles...)
	sort.Strings(dfiles)

	if len(composePaths) == 0 && len(dfiles) == 0 {
		d := diagnostic.Error(diagnostic.CodeImportEmpty, "compose path or at least one dockerfile is required")
		return Result{OK: false, Refusals: refusalsFromDiags([]diagnostic.Diagnostic{d}), Diagnostics: []diagnostic.Diagnostic{d}}
	}

	images := map[string]any{}
	services := map[string]any{}
	networks := map[string]any{}
	volumes := map[string]any{}
	resources := map[string]any{}
	parameters := map[string]any{}
	secrets := map[string]any{}
	mode := "release"
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "imported"
	}
	name = sanitizeID(name)
	if name == "" {
		name = "imported"
	}

	sources := map[string]any{}
	var composeList []any
	var secretSuggestions []SecretSuggestion
	importOpts := importOptionsFromRequest(req)
	fragments := map[string]serviceFragment{}
	put := func(bucket map[string]any, id string, val any, entity, file, pointer string) bool {
		return putEntity(bucket, id, val, entity, file, pointer, &diags, &refusals)
	}

	for _, composeRel := range composePaths {
		rel, err := validateRel(composeRel)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeImportPathInvalid, "compose path: "+err.Error()).
				WithSource(composeRel, "", 0, 0)
			diags = append(diags, d)
			continue
		}
		abs := filepath.Join(req.Workspace, filepath.FromSlash(rel))
		if !underRoot(req.Workspace, abs) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, "compose path escapes workspace").
				WithSource(rel, "", 0, 0))
			continue
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "read compose: "+err.Error()).
				WithSource(rel, "", 0, 0))
			continue
		}
		doc := loader.ParseComposeInput(rel, raw)
		if len(doc.Diags) > 0 {
			diags = append(diags, doc.Diags...)
			continue
		}
		if doc.Root == nil || doc.Root.Kind != loader.KindMap {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportParseError, "compose root must be a mapping").
				WithSource(rel, "", 1, 1))
			continue
		}
		composeList = append(composeList, rel)
		cdiags := mapCompose(doc.Root, rel, images, services, networks, volumes, resources, parameters, secrets, &mode, importOpts, put, &secretSuggestions, fragments)
		diags = append(diags, cdiags...)
	}
	if len(composeList) == 1 {
		sources["compose"] = composeList[0]
	} else if len(composeList) > 1 {
		sources["compose"] = composeList
	}
	if profs := normalizeActiveProfiles(req.ActiveProfiles); len(profs) > 0 {
		list := make([]any, len(profs))
		for i, p := range profs {
			list[i] = p
		}
		sources["activeProfiles"] = list
	}
	if req.Strict {
		sources["importStrict"] = true
	}

	for _, df := range dfiles {
		rel, err := validateRel(df)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, "dockerfile path: "+err.Error()).
				WithSource(df, "", 0, 0))
			continue
		}
		abs := filepath.Join(req.Workspace, filepath.FromSlash(rel))
		if !underRoot(req.Workspace, abs) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, "dockerfile path escapes workspace").
				WithSource(rel, "", 0, 0))
			continue
		}
		if _, err := os.Lstat(abs); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "dockerfile missing: "+rel).
				WithSource(rel, "", 0, 0))
			continue
		}
		id := imageIDFromDockerfile(rel)
		ctx, file := splitDockerfile(rel)
		img := map[string]any{
			"build": map[string]any{
				"context":    ctx,
				"dockerfile": file,
				"include":    []any{"**"},
			},
		}
		if put(images, id, img, "image", rel, "") {
			mode = "development"
			list, _ := sources["dockerfiles"].([]any)
			list = append(list, rel)
			sources["dockerfiles"] = list
		}
	}

	for sid, svc := range services {
		body, _ := svc.(map[string]any)
		if body == nil {
			delete(services, sid)
			continue
		}
		if _, hasImage := body["image"]; !hasImage {
			delete(services, sid)
		}
	}
	diags = append(diags, downgradeHealthyWithoutProbe(services)...)
	applyImageUsers(services, images, req.Workspace, req.ExistingProject, &diags)
	linkEnvironmentFrom(services, images, &diags)
	if len(services) == 0 && len(images) == 0 {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportEmpty, "no importable services or images produced"))
	}
	if mode == "release" && anyMutablePullRef(images) {
		mode = "development"
		diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
			"spec.mode set to development: at least one pull ref is not repository@sha256:<64-hex> (a tag, or a parameter with no default).").
			WithSource("", "/spec/mode", 0, 0).
			WithHint("Pin refs to digests and set mode release when you are ready to lock a release"))
	}

	project := map[string]any{
		"apiVersion": "iacpack.io/v1alpha1",
		"kind":       "Project",
		"metadata": map[string]any{
			"name":    name,
			"version": "0.1.0",
		},
		"spec": map[string]any{
			"target": map[string]any{"profile": "compose-docker/v1"},
			"mode":   mode,
		},
	}
	spec := project["spec"].(map[string]any)
	if len(parameters) > 0 {
		spec["parameters"] = sortMap(parameters)
	}
	if len(secrets) > 0 {
		spec["secrets"] = sortMap(secrets)
	}
	if len(images) > 0 {
		spec["images"] = sortMap(images)
	}
	if len(resources) > 0 {
		reconcileResourceOverlaps(resources)
		spec["resources"] = sortMap(resources)
	}
	if len(networks) > 0 {
		spec["networks"] = sortMap(networks)
	}
	if len(volumes) > 0 {
		spec["volumes"] = sortMap(volumes)
	}
	if len(services) > 0 {
		spec["services"] = sortMap(services)
	}

	sum := Summary{
		Services:   len(services),
		Images:     len(images),
		Networks:   len(networks),
		Volumes:    len(volumes),
		Resources:  len(resources),
		Parameters: len(parameters),
	}
	gaps := buildParameterGaps(parameters)
	secretSuggestions = dedupeSuggestions(secretSuggestions)
	envExample := EnvExamplePlaceholders(project, suggestionIDs(secretSuggestions))
	diags = append(diags, missingResourceSourceDiags(req.Workspace, resources)...)
	diags = append(diags, missingBuildContextDiags(req.Workspace, images)...)
	diags = append(diags, migrateGapDiagnostics(req.Workspace, services)...)
	refusals = append(refusals, refusalsFromDiags(diags)...)
	refusals = dedupeRefusals(refusals)
	sort.SliceStable(refusals, func(i, j int) bool {
		if refusals[i].Path != refusals[j].Path {
			return refusals[i].Path < refusals[j].Path
		}
		if refusals[i].Pointer != refusals[j].Pointer {
			return refusals[i].Pointer < refusals[j].Pointer
		}
		return refusals[i].Code < refusals[j].Code
	})
	hasDraft := len(services) > 0 || len(images) > 0
	return Result{
		OK:                proposeResultOK(diags, hasDraft),
		Project:           project,
		Summary:           sum,
		Sources:           sources,
		Refusals:          refusals,
		ParameterGaps:     gaps,
		SecretSuggestions: secretSuggestions,
		EnvExample:        envExample,
		Diagnostics:       diags,
	}
}

func suggestionIDs(sugs []SecretSuggestion) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range sugs {
		if s.ID == "" {
			continue
		}
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		out = append(out, s.ID)
	}
	sort.Strings(out)
	return out
}

func normalizeComposePaths(req Request) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(req.Compose)
	for _, p := range req.Composes {
		add(p)
	}
	return out
}

type putFn func(bucket map[string]any, id string, val any, entity, file, pointer string) bool

func putEntity(
	bucket map[string]any,
	id string,
	val any,
	entity, file, pointer string,
	diags *[]diagnostic.Diagnostic,
	refusals *[]Refusal,
) bool {
	if _, ok := bucket[id]; ok {
		msg := fmt.Sprintf("%s id %q already claimed; keeping first (suggested rename %s-2)", entity, id, id)
		d := diagnostic.Warning(diagnostic.CodeImportCollision, msg).WithSource(file, pointer, 0, 0)
		*diags = append(*diags, d)
		*refusals = append(*refusals, Refusal{
			Path: file, Pointer: pointer, Code: diagnostic.CodeImportCollision,
			Category: "collision", Message: msg,
		})
		return false
	}
	bucket[id] = val
	return true
}

func refusalCategory(code string) string {
	switch code {
	case diagnostic.CodeImportPolicyDenied:
		return "policy"
	case diagnostic.CodeImportSecretLike:
		return "secret"
	case diagnostic.CodeImportPathInvalid, diagnostic.CodeInputIOError:
		return "path"
	case diagnostic.CodeImportCollision:
		return "collision"
	default:
		return "unsupported"
	}
}

func refusalsFromDiags(diags []diagnostic.Diagnostic) []Refusal {
	var out []Refusal
	for _, d := range diags {
		if d.Severity == diagnostic.SeverityInfo {
			continue
		}
		path := ""
		pointer := ""
		if d.Source != nil {
			path = d.Source.File
			pointer = d.Source.Pointer
		}
		out = append(out, Refusal{
			Path: path, Pointer: pointer, Code: d.Code,
			Category: refusalCategory(d.Code), Message: d.Message,
		})
	}
	return out
}

func dedupeRefusals(in []Refusal) []Refusal {
	type key struct{ path, pointer, code, msg string }
	seen := map[key]struct{}{}
	var out []Refusal
	for _, r := range in {
		k := key{r.Path, r.Pointer, r.Code, r.Message}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}

func mapCompose(
	root *loader.Node,
	file string,
	images, services, networks, volumes, resources, parameters, secrets map[string]any,
	mode *string,
	opts importOptions,
	put putFn,
	suggestions *[]SecretSuggestion,
	fragments map[string]serviceFragment,
) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic

	for _, k := range root.MapKeys() {
		switch k {
		case "services", "networks", "volumes", "secrets", "name", "version", "x-iacpack":
			// handled / ignored intentionally
		case "configs", "extensions":
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("top-level %q omitted", k)).WithSource(file, "/"+k, 0, 0))
		default:
			if strings.HasPrefix(k, "x-") {
				continue
			}
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("top-level %q omitted", k)).WithSource(file, "/"+k, 0, 0))
		}
	}

	if secRoot := root.GetMap("secrets"); secRoot != nil && secRoot.Kind == loader.KindMap {
		for _, e := range secRoot.Map {
			id := sanitizeID(e.Key)
			if id == "" {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("secret %q skipped; invalid identifier", e.Key)).
					WithSource(file, "/secrets/"+e.Key, e.KeyLine, e.KeyColumn))
				continue
			}
			// Never read secret file contents — suggest declaration only (ADR-018).
			*suggestions = append(*suggestions, SecretSuggestion{
				ID: id, Source: "compose_secret", Path: file, Pointer: "/secrets/" + e.Key,
				Description: "Compose top-level secret (declaration stub; file contents not read)",
			})
			diags = append(diags, diagnostic.Info(diagnostic.CodeImportSecretLike,
				fmt.Sprintf("secret %q suggested as declaration stub (values not imported)", id)).
				WithSource(file, "/secrets/"+e.Key, e.KeyLine, e.KeyColumn))
		}
	}

	if n := root.GetMap("networks"); n != nil && n.Kind == loader.KindMap {
		for _, e := range n.Map {
			id := sanitizeID(e.Key)
			if id == "" {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("network %q skipped; invalid identifier", e.Key)).
					WithSource(file, "/networks/"+e.Key, e.KeyLine, e.KeyColumn))
				continue
			}
			net := map[string]any{}
			if e.Value != nil && e.Value.Kind == loader.KindMap {
				if ext := e.Value.GetMap("external"); ext != nil {
					extName := e.Key
					if nameN := e.Value.GetMap("name"); nameN != nil && nameN.Kind == loader.KindString {
						extName = nameN.Str
					}
					if ext.Kind == loader.KindBool && ext.Bool {
						// external: true (+ optional sibling name)
					} else if ext.Kind == loader.KindMap {
						if nameN := ext.GetMap("name"); nameN != nil && nameN.Kind == loader.KindString {
							extName = nameN.Str
						}
					} else if ext.Kind == loader.KindString {
						extName = ext.Str
					}
					net["external"] = map[string]any{"name": extName}
				}
			}
			_ = put(networks, id, net, "network", file, "/networks/"+e.Key)
		}
	}

	if v := root.GetMap("volumes"); v != nil && v.Kind == loader.KindMap {
		for _, e := range v.Map {
			id := sanitizeID(e.Key)
			if id == "" {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("volume %q skipped; invalid identifier", e.Key)).
					WithSource(file, "/volumes/"+e.Key, e.KeyLine, e.KeyColumn))
				continue
			}
			vol := map[string]any{"retention": "retain"}
			if e.Value != nil && e.Value.Kind == loader.KindMap {
				if ext := e.Value.GetMap("external"); ext != nil {
					extName := e.Key
					if ext.Kind == loader.KindMap {
						if nameN := ext.GetMap("name"); nameN != nil && nameN.Kind == loader.KindString {
							extName = nameN.Str
						}
					} else if ext.Kind == loader.KindString {
						extName = ext.Str
					}
					vol = map[string]any{"external": map[string]any{"name": extName}}
				}
			}
			_ = put(volumes, id, vol, "volume", file, "/volumes/"+e.Key)
		}
	}

	svcRoot := root.GetMap("services")
	if svcRoot == nil || svcRoot.Kind != loader.KindMap {
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportEmpty, "compose has no services mapping").
			WithSource(file, "/services", 0, 0))
		return diags
	}

	aliases := map[string]string{}
	for _, e := range svcRoot.Map {
		sid := sanitizeID(e.Key)
		if sid == "" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %q skipped; invalid identifier", e.Key)).
				WithSource(file, "/services/"+e.Key, e.KeyLine, e.KeyColumn))
			continue
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportParseError,
				fmt.Sprintf("service %q must be a mapping", e.Key)).
				WithSource(file, "/services/"+e.Key, e.KeyLine, e.KeyColumn))
			continue
		}
		mergeFields := composeMergeFields(e.Value)
		svcProfiles := composeServiceProfiles(e.Value)
		if !serviceProfilesActive(opts.activeProfiles, svcProfiles) {
			diags = append(diags, profileSkippedDiagnostic(
				sid, file, "/services/"+e.Key, svcProfiles, e.KeyLine, e.KeyColumn))
			continue
		}
		svc, imgID, denied, sdiags := mapService(e.Value, file, "/services/"+e.Key, sid, images, volumes, resources, parameters, secrets, mode, opts, put, suggestions)
		diags = append(diags, sdiags...)
		if denied {
			if _, exists := services[sid]; !exists {
				delete(fragments, sid)
			}
			continue
		}
		frag := serviceFragment{body: svc, fields: mergeFields}
		if imgID == "" {
			if len(mergeFields) == 0 {
				diags = append(diags, diagnostic.Error(diagnostic.CodeImportEmpty,
					fmt.Sprintf("service %q has no image or build", e.Key)).
					WithSource(file, "/services/"+e.Key, e.KeyLine, e.KeyColumn))
				continue
			}
			if existing, ok := services[sid].(map[string]any); ok {
				applyOverrideFields(existing, frag)
			}
			fragments[sid] = mergeFragment(fragments[sid], frag)
			continue
		}
		svc["image"] = imgID
		if len(svcProfiles) > 0 {
			names := make([]any, len(svcProfiles))
			for i, p := range svcProfiles {
				names[i] = p
			}
			svc["profiles"] = names
		}
		if existing, ok := services[sid].(map[string]any); ok {
			if prior, ok := fragments[sid]; ok {
				applyOverrideFields(existing, prior)
				delete(fragments, sid)
			}
			if len(mergeFields) > 0 {
				applyOverrideFields(existing, frag)
				continue
			}
			_ = put(services, sid, nil, "service", file, "/services/"+e.Key)
			continue
		}
		if prior, ok := fragments[sid]; ok {
			applyOverrideFields(svc, prior)
			delete(fragments, sid)
		}
		_ = put(services, sid, svc, "service", file, "/services/"+e.Key)
		if cn := e.Value.GetMap("container_name"); cn != nil && cn.Kind == loader.KindString {
			name := strings.TrimSpace(cn.Str)
			if name != "" && name != sid {
				aliases[name] = sid
			}
		}
	}
	rewriteContainerDNS(services, aliases)
	return diags
}

func mapService(
	n *loader.Node,
	file, pointer, sid string,
	images, volumes, resources, parameters, secrets map[string]any,
	mode *string,
	opts importOptions,
	put putFn,
	suggestions *[]SecretSuggestion,
) (svc map[string]any, imgID string, denied bool, diags []diagnostic.Diagnostic) {
	svc = map[string]any{}
	for _, e := range n.Map {
		k := e.Key
		if k == "env_file" {
			// Values never loaded; suggest secret ids from basenames (ADR-018).
			continue
		}
		if k == "mem_limit" {
			continue
		}
		if code, ok := deniedServiceKeys[k]; ok {
			sev := diagnostic.SeverityWarning
			if code == diagnostic.CodeImportPolicyDenied && opts.strict {
				sev = diagnostic.SeverityError
			}
			if code == diagnostic.CodeImportUnsupportedField {
				sev = diagnostic.SeverityWarning
			}
			d := diagnostic.Diagnostic{
				Code: code, Severity: sev,
				Message:   fmt.Sprintf("service %s: %q refused", sid, k),
				Retryable: false,
			}.WithSource(file, pointer+"/"+k, e.KeyLine, e.KeyColumn)
			diags = append(diags, d)
			if code == diagnostic.CodeImportPolicyDenied {
				denied = true
			}
			continue
		}
		if warnOmitServiceKeys[k] {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: %q omitted", sid, k)).
				WithSource(file, pointer+"/"+k, e.KeyLine, e.KeyColumn))
			continue
		}
		if !allowedServiceKeys[k] && k != "env_file" && !strings.HasPrefix(k, "x-") {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: unknown field %q omitted", sid, k)).
				WithSource(file, pointer+"/"+k, e.KeyLine, e.KeyColumn))
		}
	}
	if denied {
		return nil, "", true, diags
	}

	if ef := n.GetMap("env_file"); ef != nil {
		paths := envFilePaths(ef)
		for _, pth := range paths {
			id := secretIDFromEnvFile(pth)
			if id == "" {
				id = sanitizeID(sid + "-env")
			}
			msg := fmt.Sprintf("service %s: env_file %q values not imported; secret %q", sid, pth, id)
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportSecretLike, msg).
				WithSource(file, pointer+"/env_file", 0, 0))
			desc := "Imported env_file reference (file contents not read)"
			ensureSecret(secrets, id, desc)
			*suggestions = append(*suggestions, SecretSuggestion{
				ID: id, Source: "env_file", Path: file, Pointer: pointer + "/env_file",
				EnvFile: pth, Service: sid,
				Description: desc,
			})
		}
	}

	if imgN := n.GetMap("image"); imgN != nil && imgN.Kind == loader.KindString {
		ref := imgN.Str
		literal, pid, okRef := classifyImportImageRef(ref)
		if !okRef {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportInterpolation,
				fmt.Sprintf("service %s: interpolated image ref refused", sid)).
				WithSource(file, pointer+"/image", imgN.Line, imgN.Column))
			return nil, "", true, diags
		}
		imgID = sanitizeID(sid)
		if pid != "" {
			ensureParam(parameters, pid, "string")
			diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
				fmt.Sprintf("service %s: image %s mapped to parameter %q (no default)", sid, ref, pid)).
				WithSource(file, pointer+"/image", imgN.Line, imgN.Column))
			if _, exists := images[imgID]; !exists {
				_ = put(images, imgID, map[string]any{
					"ref":      map[string]any{"parameter": pid},
					"delivery": "pull",
				}, "image", file, pointer+"/image")
			}
		} else {
			if literal != ref {
				diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
					fmt.Sprintf("service %s: image ${…:-default} resolved to fixed ref for import", sid)).
					WithSource(file, pointer+"/image", imgN.Line, imgN.Column))
			}
			if _, exists := images[imgID]; !exists {
				_ = put(images, imgID, map[string]any{"ref": literal, "delivery": "pull"}, "image", file, pointer+"/image")
			}
		}
	}

	if b := n.GetMap("build"); b != nil {
		*mode = "development"
		ctx := "."
		df := "Dockerfile"
		argMap := map[string]any{}
		if b.Kind == loader.KindString {
			ctx = b.Str
		} else if b.Kind == loader.KindMap {
			if c := b.GetMap("context"); c != nil && c.Kind == loader.KindString {
				ctx = c.Str
			}
			if d := b.GetMap("dockerfile"); d != nil && d.Kind == loader.KindString {
				df = d.Str
			}
			if args := b.GetMap("args"); args != nil && args.Kind == loader.KindMap {
				for _, ae := range args.Map {
					vs, vd := valueSource(ae.Value, file, pointer+"/build/args/"+ae.Key, parameters)
					diags = append(diags, vd...)
					if vs != nil {
						argMap[ae.Key] = vs
					}
				}
			}
			for _, bk := range b.MapKeys() {
				switch bk {
				case "context", "dockerfile", "args":
				default:
					diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
						fmt.Sprintf("service %s: build.%s omitted", sid, bk)).
						WithSource(file, pointer+"/build/"+bk, 0, 0))
				}
			}
		}
		resolvedCtx, rebased, err := resolveComposeWorkspacePath(file, ctx)
		if err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("service %s: invalid build context %q", sid, ctx)).
				WithSource(file, pointer+"/build", 0, 0))
			return nil, "", true, diags
		}
		if rebased {
			diags = append(diags, composePathRebaseDiag(file, pointer+"/build/context", ctx, resolvedCtx))
		}
		ctx = resolvedCtx
		if _, err := path.ParseWorkspacePath(ctx); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("service %s: invalid build context %q", sid, ctx)).
				WithSource(file, pointer+"/build", 0, 0))
			return nil, "", true, diags
		}
		imgID = sanitizeID(sid)
		build := map[string]any{
			"context":    ctx,
			"dockerfile": df,
			"include":    []any{"**"},
		}
		if len(argMap) > 0 {
			build["args"] = argMap
		}
		if _, exists := images[imgID]; !exists {
			_ = put(images, imgID, map[string]any{"build": build}, "image", file, pointer+"/build")
		} else {
			// Prefer build if first claim was pull from same id — keep first (ADR-017).
		}
	}

	if cmd := n.GetMap("command"); cmd != nil {
		arr, cmdDiags := importCommandValue(cmd, file, pointer+"/command", "service "+sid+" command", parameters, secrets)
		diags = append(diags, cmdDiags...)
		if len(arr) > 0 || (composeMergeReplace(cmd.Tag) && cmd.Kind == loader.KindSeq) {
			if arr == nil {
				arr = []any{}
			}
			svc["command"] = arr
		}
	}
	if ep := n.GetMap("entrypoint"); ep != nil {
		arr, epDiags := importCommandValue(ep, file, pointer+"/entrypoint", "service "+sid+" entrypoint", parameters, secrets)
		diags = append(diags, epDiags...)
		if len(arr) > 0 || (composeMergeReplace(ep.Tag) && ep.Kind == loader.KindSeq) {
			if arr == nil {
				arr = []any{}
			}
			svc["entrypoint"] = arr
		}
	}
	if u := n.GetMap("user"); u != nil && u.Kind == loader.KindString {
		user, ud := importServiceUser(u.Str, file, pointer+"/user")
		diags = append(diags, ud...)
		if user != "" {
			svc["user"] = user
		}
	}

	if env := n.GetMap("environment"); env != nil {
		envMap := map[string]any{}
		switch env.Kind {
		case loader.KindMap:
			for _, ee := range env.Map {
				raw := ""
				if ee.Value != nil && ee.Value.Kind == loader.KindString {
					raw = ee.Value.Str
				}
				if interpRe.MatchString(raw) {
					vs, vd := envInterpValue(raw, file, pointer+"/environment/"+ee.Key, parameters, secrets)
					diags = append(diags, vd...)
					if vs != nil {
						envMap[ee.Key] = vs
						for _, id := range secretIDsInEnvValue(vs) {
							*suggestions = append(*suggestions, SecretSuggestion{
								ID: id, Source: "env_key", Path: file,
								Pointer: pointer + "/environment/" + ee.Key,
								EnvKey:  ee.Key, Service: sid,
								Target:      "/run/secrets/" + id,
								Description: "Suggested from secret interpolation (value not imported)",
							})
						}
					}
					continue
				}
				if isSecretLikeKey(ee.Key) || isSecretInterp(raw) {
					id := secretIDFromEnvRaw(ee.Key, raw)
					diags = append(diags, diagnostic.Warning(diagnostic.CodeImportSecretLike,
						fmt.Sprintf("service %s: env key %q mapped to secret %q (value not imported)", sid, ee.Key, id)).
						WithSource(file, pointer+"/environment/"+ee.Key, ee.KeyLine, ee.KeyColumn))
					ensureSecret(secrets, id, "Imported secret reference (value not imported)")
					envMap[ee.Key] = map[string]any{"secret": id}
					*suggestions = append(*suggestions, SecretSuggestion{
						ID: id, Source: "env_key", Path: file,
						Pointer: pointer + "/environment/" + ee.Key,
						EnvKey:  ee.Key, Service: sid,
						Target:      "/run/secrets/" + id,
						Description: "Suggested from secret-like environment key (value not imported)",
					})
					continue
				}
				vs, vd := valueSource(ee.Value, file, pointer+"/environment/"+ee.Key, parameters)
				diags = append(diags, vd...)
				if vs != nil {
					envMap[ee.Key] = vs
				}
			}
		case loader.KindSeq:
			for i, ee := range env.Seq {
				if ee.Kind != loader.KindString {
					continue
				}
				key, valNode, ok := envListEntry(ee.Str)
				if !ok {
					diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
						fmt.Sprintf("service %s: environment[%d] omitted; expected KEY=VALUE", sid, i)).
						WithSource(file, pointer+"/environment", ee.Line, ee.Column))
					continue
				}
				if interpRe.MatchString(valNode) {
					vs, vd := envInterpValue(valNode, file, pointer+"/environment", parameters, secrets)
					diags = append(diags, vd...)
					if vs != nil {
						envMap[key] = vs
						for _, id := range secretIDsInEnvValue(vs) {
							*suggestions = append(*suggestions, SecretSuggestion{
								ID: id, Source: "env_key", Path: file,
								Pointer: pointer + "/environment",
								EnvKey:  key, Service: sid,
								Target:      "/run/secrets/" + id,
								Description: "Suggested from secret interpolation (value not imported)",
							})
						}
					}
					continue
				}
				if isSecretLikeKey(key) || isSecretInterp(valNode) {
					id := secretIDFromEnvRaw(key, valNode)
					diags = append(diags, diagnostic.Warning(diagnostic.CodeImportSecretLike,
						fmt.Sprintf("service %s: env key %q mapped to secret %q (value not imported)", sid, key, id)).
						WithSource(file, pointer+"/environment", ee.Line, ee.Column))
					ensureSecret(secrets, id, "Imported secret reference (value not imported)")
					envMap[key] = map[string]any{"secret": id}
					*suggestions = append(*suggestions, SecretSuggestion{
						ID: id, Source: "env_key", Path: file,
						Pointer: pointer + "/environment",
						EnvKey:  key, Service: sid,
						Target:      "/run/secrets/" + id,
						Description: "Suggested from secret-like environment key (value not imported)",
					})
					continue
				}
				vs, vd := valueSource(&loader.Node{Kind: loader.KindString, Str: valNode, Line: ee.Line, Column: ee.Column, File: file},
					file, pointer+"/environment", parameters)
				diags = append(diags, vd...)
				if vs != nil {
					envMap[key] = vs
				}
			}
		}
		if len(envMap) > 0 {
			svc["environment"] = envMap
		}
	}

	if ports := n.GetMap("ports"); ports != nil && ports.Kind == loader.KindSeq {
		var out []any
		for i, p := range ports.Seq {
			pr, pd := mapPort(p, file, fmt.Sprintf("%s/ports/%d", pointer, i), parameters)
			diags = append(diags, pd...)
			if pr != nil {
				out = append(out, pr)
			}
		}
		if len(out) > 0 {
			svc["ports"] = out
		}
	}

	if nets := n.GetMap("networks"); nets != nil {
		var list []any
		switch nets.Kind {
		case loader.KindSeq:
			for _, nn := range nets.Seq {
				if nn.Kind == loader.KindString {
					id := sanitizeID(nn.Str)
					if id != "" {
						list = append(list, id)
					}
				}
			}
		case loader.KindMap:
			for _, ne := range nets.Map {
				id := sanitizeID(ne.Key)
				if id != "" {
					list = append(list, id)
				}
			}
		}
		if len(list) > 0 {
			svc["networks"] = list
		}
	}

	if vols := n.GetMap("volumes"); vols != nil && (vols.Kind == loader.KindSeq || composeMergeReplace(vols.Tag)) {
		var mounts []any
		if vols.Kind == loader.KindSeq {
			for i, v := range vols.Seq {
				m, md := mapMount(v, file, fmt.Sprintf("%s/volumes/%d", pointer, i), sid, volumes, resources, parameters, opts.strict)
				diags = append(diags, md...)
				if m != nil {
					mounts = append(mounts, m)
				}
			}
		}
		if len(mounts) > 0 || composeMergeReplace(vols.Tag) {
			if mounts == nil {
				mounts = []any{}
			}
			svc["mounts"] = mounts
		}
	}

	if deps := n.GetMap("depends_on"); deps != nil {
		var out []any
		switch deps.Kind {
		case loader.KindSeq:
			for _, d := range deps.Seq {
				if d.Kind == loader.KindString {
					tid := sanitizeID(d.Str)
					if tid != "" {
						out = append(out, map[string]any{"target": tid, "condition": "started"})
					}
				}
			}
		case loader.KindMap:
			for _, de := range deps.Map {
				tid := sanitizeID(de.Key)
				cond := "started"
				if de.Value != nil && de.Value.Kind == loader.KindMap {
					if c := de.Value.GetMap("condition"); c != nil && c.Kind == loader.KindString {
						cond = mapCondition(c.Str)
					}
				}
				if tid != "" {
					out = append(out, map[string]any{"target": tid, "condition": cond})
				}
			}
		}
		if len(out) > 0 {
			svc["dependsOn"] = out
		}
	}

	if hc := n.GetMap("healthcheck"); hc != nil && hc.Kind == loader.KindMap {
		h := map[string]any{}
		if t := hc.GetMap("test"); t != nil {
			if arr, hcDiags := importHealthcheckCommand(t, file, pointer, sid); len(arr) > 0 {
				diags = append(diags, hcDiags...)
				h["command"] = arr
			} else if t.Kind == loader.KindSeq {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("service %s: healthcheck omitted (Compose ${…} in test command)", sid)).
					WithSource(file, pointer+"/healthcheck/test", 0, 0))
			}
		}
		if iv := hc.GetMap("interval"); iv != nil && iv.Kind == loader.KindString {
			h["interval"] = iv.Str
		}
		if to := hc.GetMap("timeout"); to != nil && to.Kind == loader.KindString {
			h["timeout"] = to.Str
		}
		if sp := hc.GetMap("start_period"); sp != nil && sp.Kind == loader.KindString {
			h["startPeriod"] = sp.Str
		}
		if r := hc.GetMap("retries"); r != nil && r.Kind == loader.KindNumber {
			if n, err := strconv.Atoi(r.Str); err == nil {
				h["retries"] = n
			}
		}
		if _, ok := h["command"]; ok {
			svc["healthcheck"] = h
		}
	}

	if rs := n.GetMap("restart"); rs != nil && rs.Kind == loader.KindString {
		svc["restart"] = rs.Str
	}

	if dep := n.GetMap("deploy"); dep != nil && dep.Kind == loader.KindMap {
		if res := dep.GetMap("resources"); res != nil {
			if lim := res.GetMap("limits"); lim != nil && lim.Kind == loader.KindMap {
				limits := map[string]any{}
				if c := lim.GetMap("cpus"); c != nil && c.Kind == loader.KindString {
					if normalized, ok := v1alpha1.NormalizeResourceCPUs(c.Str); ok {
						limits["cpus"] = normalized
					} else {
						diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
							fmt.Sprintf("service %s: deploy.resources.limits.cpus %q omitted; not a valid manifest CPU limit", sid, c.Str)).
							WithSource(file, pointer+"/deploy/resources/limits/cpus", c.Line, c.Column))
					}
				}
				if m := lim.GetMap("memory"); m != nil {
					if b, ok, md := composeMemoryBytesFromNode(m); ok {
						diags = append(diags, md...)
						limits["memory"] = b
					} else {
						diags = append(diags, md...)
					}
				}
				gpu, all, gd := importDeployGPU(res, file, pointer, sid)
				diags = append(diags, gd...)
				assignImportedGPU(limits, gpu, all)
				if len(limits) > 0 {
					svc["resources"] = limits
				}
			} else if gpu, all, gd := importDeployGPU(res, file, pointer, sid); gpu > 0 || all || len(gd) > 0 {
				diags = append(diags, gd...)
				if gpu > 0 || all {
					limits := map[string]any{}
					assignImportedGPU(limits, gpu, all)
					svc["resources"] = limits
				}
			}
		}
		for _, dk := range dep.MapKeys() {
			if dk != "resources" {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					fmt.Sprintf("service %s: deploy.%s omitted", sid, dk)).
					WithSource(file, pointer+"/deploy/"+dk, 0, 0))
			}
		}
	}

	if ml := n.GetMap("mem_limit"); ml != nil {
		applyServiceMemoryLimit(svc, sid, file, pointer+"/mem_limit", ml, &diags)
	}

	if sec := n.GetMap("secrets"); sec != nil {
		switch sec.Kind {
		case loader.KindSeq:
			for i, item := range sec.Seq {
				id, target := parseServiceSecret(item, sid)
				if id == "" {
					diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
						fmt.Sprintf("service %s: secrets[%d] omitted; invalid id", sid, i)).
						WithSource(file, fmt.Sprintf("%s/secrets/%d", pointer, i), 0, 0))
					continue
				}
				if target == "" {
					target = "/run/secrets/" + id
				}
				*suggestions = append(*suggestions, SecretSuggestion{
					ID: id, Source: "compose_secret", Path: file,
					Pointer: fmt.Sprintf("%s/secrets/%d", pointer, i),
					Service: sid, Target: target,
					Description: "Compose service secret mount (declaration stub; values not imported)",
				})
				diags = append(diags, diagnostic.Info(diagnostic.CodeImportSecretLike,
					fmt.Sprintf("service %s: secret mount %q suggested (values not imported)", sid, id)).
					WithSource(file, fmt.Sprintf("%s/secrets/%d", pointer, i), 0, 0))
			}
		case loader.KindMap:
			for _, se := range sec.Map {
				id := sanitizeID(se.Key)
				if id == "" {
					continue
				}
				target := "/run/secrets/" + id
				*suggestions = append(*suggestions, SecretSuggestion{
					ID: id, Source: "compose_secret", Path: file,
					Pointer: pointer + "/secrets/" + se.Key,
					Service: sid, Target: target,
					Description: "Compose service secret mount (declaration stub; values not imported)",
				})
			}
		default:
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: secrets mounts omitted", sid)).
				WithSource(file, pointer+"/secrets", 0, 0))
		}
	}

	return svc, imgID, false, diags
}

func composePathRebaseDiag(file, pointer, composeRel, workspaceRel string) diagnostic.Diagnostic {
	return diagnostic.Warning(diagnostic.CodeImportPathInvalid,
		fmt.Sprintf("compose path %q rebased to workspace %q (compose file not in a subfolder)", composeRel, workspaceRel)).
		WithSource(file, pointer, 0, 0).
		WithHint("Prefer placing the compose file under a subfolder (e.g. docker/stack/) or keep this rebased path")
}

func envListEntry(s string) (key, value string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:eq]), strings.TrimSpace(s[eq+1:]), true
}

func envFilePaths(n *loader.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	switch n.Kind {
	case loader.KindString:
		out = append(out, n.Str)
	case loader.KindSeq:
		for _, c := range n.Seq {
			if c.Kind == loader.KindString {
				out = append(out, c.Str)
			}
		}
	}
	return out
}

func parseServiceSecret(n *loader.Node, sid string) (id, target string) {
	if n == nil {
		return "", ""
	}
	switch n.Kind {
	case loader.KindString:
		return sanitizeID(n.Str), "/run/secrets/" + sanitizeID(n.Str)
	case loader.KindMap:
		src := ""
		if s := n.GetMap("source"); s != nil && s.Kind == loader.KindString {
			src = s.Str
		}
		if src == "" {
			return "", ""
		}
		id = sanitizeID(src)
		target = "/run/secrets/" + id
		if t := n.GetMap("target"); t != nil && t.Kind == loader.KindString && t.Str != "" {
			target = t.Str
		}
		_ = sid
		return id, target
	default:
		return "", ""
	}
}

func mapPort(n *loader.Node, file, pointer string, parameters map[string]any) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if n.Kind == loader.KindString {
		return mapShortPortString(n.Str, file, pointer, parameters, n.Line, n.Column)
	}
	if n.Kind != loader.KindMap {
		return nil, diags
	}
	out := map[string]any{}
	if t := n.GetMap("target"); t != nil && t.Kind == loader.KindNumber {
		if v, err := strconv.Atoi(t.Str); err == nil {
			out["container"] = v
		}
	}
	if p := n.GetMap("published"); p != nil {
		switch p.Kind {
		case loader.KindNumber:
			if v, err := strconv.Atoi(p.Str); err == nil {
				out["host"] = v
			}
		case loader.KindString:
			if interpRe.MatchString(p.Str) {
				id := sanitizeID(paramIDFromInterp(p.Str))
				if id == "" {
					id = "port"
				}
				ensureParam(parameters, id, "integer")
				out["host"] = map[string]any{"parameter": id}
				diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
					fmt.Sprintf("%s: published port mapped to parameter %q", pointer, id)).
					WithSource(file, pointer+"/published", p.Line, p.Column))
			} else if v, err := strconv.Atoi(p.Str); err == nil {
				out["host"] = v
			}
		}
	}
	if b := n.GetMap("host_ip"); b != nil && b.Kind == loader.KindString {
		out["bind"] = b.Str
	}
	if proto := n.GetMap("protocol"); proto != nil && proto.Kind == loader.KindString {
		out["protocol"] = proto.Str
	}
	if _, ok := out["container"]; !ok {
		return nil, diags
	}
	return out, diags
}

func mapMount(
	n *loader.Node,
	file, pointer, sid string,
	volumes, resources, parameters map[string]any,
	strict bool,
) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if n.Kind == loader.KindString {
		src, tgt, mode, ok := splitVolumeShort(n.Str)
		if !ok {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				"string-form volume omitted; expected SRC:TARGET[:MODE]").
				WithSource(file, pointer, n.Line, n.Column))
			return nil, diags
		}
		ro := volumeModeReadOnly(mode)
		if looksLikeBindSource(src) {
			return mountBindFromSource(file, sid, file, pointer, src, tgt, ro, resources, parameters, strict)
		}
		vid := sanitizeID(src)
		if vid == "" {
			return nil, diags
		}
		if _, err := path.ParseContainerPath(tgt); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("invalid mount target %q", tgt)).
				WithSource(file, pointer, n.Line, n.Column))
			return nil, diags
		}
		if _, ok := volumes[vid]; !ok {
			volumes[vid] = map[string]any{"retention": "retain"}
		}
		m := map[string]any{"volume": vid, "target": tgt, "readOnly": ro}
		return m, diags
	}
	if n.Kind != loader.KindMap {
		return nil, diags
	}
	typ := "volume"
	if t := n.GetMap("type"); t != nil && t.Kind == loader.KindString {
		typ = t.Str
	}
	src := ""
	if s := n.GetMap("source"); s != nil && s.Kind == loader.KindString {
		src = s.Str
	}
	tgt := ""
	if t := n.GetMap("target"); t != nil && t.Kind == loader.KindString {
		tgt = t.Str
	}
	ro := false
	if r := n.GetMap("read_only"); r != nil && r.Kind == loader.KindBool {
		ro = r.Bool
	}
	if tgt == "" {
		return nil, diags
	}
	if _, err := path.ParseContainerPath(tgt); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
			fmt.Sprintf("invalid mount target %q", tgt)).
			WithSource(file, pointer+"/target", 0, 0))
		return nil, diags
	}
	switch typ {
	case "volume":
		vid := sanitizeID(src)
		if vid == "" {
			return nil, diags
		}
		if _, ok := volumes[vid]; !ok {
			volumes[vid] = map[string]any{"retention": "retain"}
		}
		m := map[string]any{"volume": vid, "target": tgt, "readOnly": ro}
		return m, diags
	case "bind":
		return mountBindFromSource(file, sid, file, pointer+"/source", src, tgt, ro, resources, parameters, strict)
	default:
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("mount type %q omitted", typ)).
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
}

func valueSource(n *loader.Node, file, pointer string, parameters map[string]any) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if n == nil || n.Kind == loader.KindNull {
		return map[string]any{"literal": ""}, diags
	}
	switch n.Kind {
	case loader.KindBool:
		return map[string]any{"literal": n.Bool}, diags
	case loader.KindNumber:
		if strings.Contains(n.Str, ".") {
			f, _ := strconv.ParseFloat(n.Str, 64)
			return map[string]any{"literal": f}, diags
		}
		i, _ := strconv.Atoi(n.Str)
		return map[string]any{"literal": i}, diags
	case loader.KindString:
		if interpRe.MatchString(n.Str) {
			name := paramIDFromInterp(n.Str)
			id := sanitizeID(name)
			if id == "" {
				id = "param"
			}
			if isSecretLikeKey(name) {
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportSecretLike,
					fmt.Sprintf("interpolated secret-like value at %s omitted", pointer)).
					WithSource(file, pointer, n.Line, n.Column))
				return nil, diags
			}
			ensureParam(parameters, id, "string")
			diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
				fmt.Sprintf("%s: ${…} mapped to parameter %q (no default)", pointer, id)).
				WithSource(file, pointer, n.Line, n.Column))
			return map[string]any{"parameter": id}, diags
		}
		return map[string]any{"literal": n.Str}, diags
	default:
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			"unsupported value type omitted").WithSource(file, pointer, n.Line, n.Column))
		return nil, diags
	}
}

func isSecretLikeKey(key string) bool {
	if numericSettingRe.MatchString(key) {
		return false
	}
	return secretNameRe.MatchString(key)
}

// isSecretInterp reports a whole-value ${VAR} whose variable name is secret-like.
// The environment key may differ from VAR (RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD}).
func isSecretInterp(raw string) bool {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") || strings.Count(raw, "${") != 1 {
		return false
	}
	return isSecretLikeKey(paramIDFromInterp(raw))
}

func secretIDFromEnvRaw(key, raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "${") && strings.HasSuffix(raw, "}") && strings.Count(raw, "${") == 1 {
		if id := sanitizeID(paramIDFromInterp(raw)); id != "" {
			return id
		}
	}
	if id := sanitizeID(key); id != "" {
		return id
	}
	return "secret"
}

func secretIDFromEnvFile(pth string) string {
	p := filepath.ToSlash(strings.TrimSpace(pth))
	p = strings.TrimPrefix(p, "./")
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
	p = strings.ReplaceAll(p, "/", "-")
	p = strings.ReplaceAll(p, ".", "-")
	return sanitizeID(p)
}

func ensureSecret(secrets map[string]any, id, desc string) {
	if secrets == nil || id == "" {
		return
	}
	if _, ok := secrets[id]; ok {
		return
	}
	secrets[id] = map[string]any{
		"required":    true,
		"delivery":    "file",
		"description": desc,
	}
}

func ensureParam(parameters map[string]any, id, typ string) {
	if parameters == nil {
		return
	}
	if _, ok := parameters[id]; ok {
		return
	}
	parameters[id] = map[string]any{
		"phase": "deploy",
		"type":  typ,
	}
}

func paramIDFromInterp(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "${")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimPrefix(s, "$")
	if i := strings.IndexAny(s, ":-"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

// downgradeHealthyWithoutProbe rewrites condition healthy when the target
// service has no healthcheck, so propose does not emit a draft validate rejects.
func downgradeHealthyWithoutProbe(services map[string]any) []diagnostic.Diagnostic {
	ids := make([]string, 0, len(services))
	hasProbe := map[string]bool{}
	for id, raw := range services {
		ids = append(ids, id)
		body, _ := raw.(map[string]any)
		if body == nil {
			continue
		}
		if _, ok := body["healthcheck"]; ok {
			hasProbe[id] = true
		}
	}
	sort.Strings(ids)
	var diags []diagnostic.Diagnostic
	for _, id := range ids {
		body, _ := services[id].(map[string]any)
		if body == nil {
			continue
		}
		deps, _ := body["dependsOn"].([]any)
		for i, raw := range deps {
			dep, _ := raw.(map[string]any)
			if dep == nil || dep["condition"] != "healthy" {
				continue
			}
			target, _ := dep["target"].(string)
			if target != "" && hasProbe[target] {
				continue
			}
			dep["condition"] = "started"
			deps[i] = dep
			msg := fmt.Sprintf("service %s: depends_on %s condition healthy downgraded to started (target has no healthcheck)", id, target)
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField, msg).
				WithSource("", "/services/"+id+"/dependsOn", 0, 0).
				WithHint("service_healthy needs a healthcheck the manifest can store"))
		}
	}
	return diags
}

func mapCondition(c string) string {
	switch c {
	case "service_healthy", "healthy":
		return "healthy"
	case "service_completed_successfully", "completedSuccessfully":
		return "completedSuccessfully"
	default:
		return "started"
	}
}

func stringList(n *loader.Node) []any {
	if n == nil {
		return nil
	}
	if n.Kind != loader.KindSeq {
		return nil
	}
	out := make([]any, 0, len(n.Seq))
	for _, c := range n.Seq {
		if c.Kind == loader.KindString {
			out = append(out, c.Str)
		} else if c.Kind == loader.KindNumber || c.Kind == loader.KindBool {
			out = append(out, c.Str)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeID(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "x-" + s
	}
	if len(s) > 63 {
		s = s[:63]
		s = strings.Trim(s, "-")
	}
	if !idRe.MatchString(s) {
		return ""
	}
	return s
}

func imageIDFromDockerfile(rel string) string {
	base := filepath.Base(rel)
	dir := filepath.Base(filepath.Dir(rel))
	if dir == "." || dir == "/" || dir == "" {
		return sanitizeID(strings.TrimPrefix(base, "Dockerfile"))
	}
	id := sanitizeID(dir)
	if id == "" {
		id = "image"
	}
	return id
}

func splitDockerfile(rel string) (context, dockerfile string) {
	rel = filepath.ToSlash(rel)
	slash := strings.LastIndex(rel, "/")
	if slash <= 0 {
		return ".", rel
	}
	return rel[:slash], rel[slash+1:]
}

func validateRel(p string) (string, error) {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if err := inputs.ValidateRelPath(p); err != nil {
		return "", err
	}
	if _, err := path.ParseWorkspacePath(p); err != nil {
		return "", err
	}
	return p, nil
}

func underRoot(root, abs string) bool {
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func sortMap(m map[string]any) map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(m))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}
