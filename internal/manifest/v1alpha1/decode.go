// Package v1alpha1 converts loader trees into domain Projects.
// DTOs stay inside this package and must not leak into domain/app.
package v1alpha1

import (
	"fmt"
	"regexp"
	"strings"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/path"
	"iacpack/internal/manifest/loader"
)

var (
	identifierRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	profileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	semverRe      = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	durationRe    = regexp.MustCompile(`^([1-9]\d*)(ms|s|m|h)$`)
	modeOctalRe   = regexp.MustCompile(`^0[0-7]{3}$`)
	cpusRe        = regexp.MustCompile(`^(0\.(00[1-9]|0[1-9]\d|[1-9]\d{0,2})|[1-9]\d{0,3}|10[0-1]\d|102[0-4])$`)
	sha256IDRe    = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	buildArgRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	scriptNameRe  = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,64}$`)
	dbNameRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
)

const (
	maxDescriptionBytes = 4096
	maxPatternBytes     = 1024
	maxDuration         = 24 * 60 * 60 * 1000 // ms
	minMemoryBytes      = 1 << 20
	maxMemoryBytes      = 1 << 40
)

// Decode validates the loader tree against the full v1alpha1 contract.
func Decode(doc loader.Document) (domain.Project, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if len(doc.Diags) > 0 {
		return domain.Project{}, append([]diagnostic.Diagnostic(nil), doc.Diags...)
	}
	if doc.Root == nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeManifestParseError, "empty document").
			WithSource(doc.File, "", 1, 1))
		return domain.Project{}, diags
	}
	if doc.Root.Kind != loader.KindMap {
		diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidType, "manifest root must be a mapping").
			WithSource(doc.File, "", doc.Root.Line, doc.Root.Column))
		return domain.Project{}, diags
	}

	file := doc.File
	root := doc.Root
	diags = append(diags, rejectUnknown(file, "", root, map[string]struct{}{
		"apiVersion": {}, "kind": {}, "metadata": {}, "spec": {},
	})...)

	apiVersion, d := requireString(file, root, "apiVersion", "/apiVersion")
	diags = append(diags, d...)
	kind, d := requireString(file, root, "kind", "/kind")
	diags = append(diags, d...)

	if apiVersion != "" && apiVersion != domain.APIVersionV1Alpha1 {
		n := root.GetMap("apiVersion")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestUnsupportedAPI,
			fmt.Sprintf("unsupported apiVersion %q", apiVersion)), file, "/apiVersion", n).
			WithHint("Supported apiVersion: iacpack.io/v1alpha1"))
	}
	if kind != "" && kind != domain.KindProject {
		n := root.GetMap("kind")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestUnsupportedKind,
			fmt.Sprintf("unsupported kind %q", kind)), file, "/kind", n).
			WithHint("Supported kind: Project"))
	}

	meta, d := decodeMetadata(file, root.GetMap("metadata"))
	diags = append(diags, d...)
	spec, d := decodeSpec(file, root.GetMap("spec"))
	diags = append(diags, d...)

	project := domain.Project{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata:   meta,
		Spec:       spec,
	}
	if !diagnostic.HasError(diags) {
		diags = append(diags, semanticValidate(file, project)...)
	}
	diagnostic.SortDeterministic(diags)
	if diagnostic.HasError(diags) {
		return domain.Project{}, diags
	}
	return project, diags
}

func decodeMetadata(file string, node *loader.Node) (domain.Metadata, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var meta domain.Metadata
	if node == nil {
		diags = append(diags, missing(file, "/metadata", nil))
		return meta, diags
	}
	if node.Kind != loader.KindMap {
		return meta, []diagnostic.Diagnostic{typeErr(file, "/metadata", "object", node)}
	}
	diags = append(diags, rejectUnknown(file, "/metadata", node, map[string]struct{}{
		"name": {}, "version": {}, "description": {},
	})...)
	name, d := requireString(file, node, "name", "/metadata/name")
	diags = append(diags, d...)
	version, d := requireString(file, node, "version", "/metadata/version")
	diags = append(diags, d...)
	if name != "" && !identifierRe.MatchString(name) {
		n := node.GetMap("name")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
			fmt.Sprintf("metadata.name %q is not a valid identifier", name)),
			file, "/metadata/name", n).WithHint("Use ^[a-z][a-z0-9-]{0,62}$"))
	}
	if version != "" {
		if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") || !semverRe.MatchString(version) {
			n := node.GetMap("version")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidSemver,
				fmt.Sprintf("metadata.version %q is not a valid SemVer without a v prefix", version)),
				file, "/metadata/version", n))
		}
	}
	meta.Name, meta.Version = name, version
	if descNode := node.GetMap("description"); descNode != nil {
		if descNode.Kind != loader.KindString {
			diags = append(diags, typeErr(file, "/metadata/description", "string", descNode))
		} else if len(descNode.Str) > maxDescriptionBytes {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"metadata.description exceeds 4 KiB"), file, "/metadata/description", descNode))
		} else {
			meta.Description = descNode.Str
		}
	}
	return meta, diags
}

func decodeSpec(file string, node *loader.Node) (domain.Spec, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var spec domain.Spec
	if node == nil {
		diags = append(diags, missing(file, "/spec", nil))
		return spec, diags
	}
	if node.Kind != loader.KindMap {
		return spec, []diagnostic.Diagnostic{typeErr(file, "/spec", "object", node)}
	}
	diags = append(diags, rejectUnknown(file, "/spec", node, map[string]struct{}{
		"target": {}, "mode": {}, "parameters": {}, "secrets": {}, "images": {},
		"builds": {}, "resources": {}, "networks": {}, "volumes": {}, "services": {}, "tasks": {},
		"databases": {},
		"package": {}, "x-ui": {},
	})...)

	targetNode := node.GetMap("target")
	if targetNode == nil {
		diags = append(diags, missing(file, "/spec/target", node))
	} else if targetNode.Kind != loader.KindMap {
		diags = append(diags, typeErr(file, "/spec/target", "object", targetNode))
	} else {
		diags = append(diags, rejectUnknown(file, "/spec/target", targetNode, map[string]struct{}{"profile": {}})...)
		profile, d := requireString(file, targetNode, "profile", "/spec/target/profile")
		diags = append(diags, d...)
		if profile != "" && profile != string(domain.ProfileComposeDockerV1) {
			n := targetNode.GetMap("profile")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestUnsupportedProfile,
				fmt.Sprintf("unsupported profile %q", profile)), file, "/spec/target/profile", n).
				WithHint("Supported profile: compose-docker/v1"))
		}
		spec.Target = domain.Target{Profile: domain.Profile(profile)}
	}

	mode, d := requireString(file, node, "mode", "/spec/mode")
	diags = append(diags, d...)
	if mode != "" && mode != string(domain.ModeRelease) && mode != string(domain.ModeDevelopment) {
		n := node.GetMap("mode")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			fmt.Sprintf("unsupported mode %q", mode)), file, "/spec/mode", n).
			WithHint("Supported modes: development, release"))
	}
	spec.Mode = domain.Mode(mode)

	params, d := decodeParameters(file, node.GetMap("parameters"))
	diags = append(diags, d...)
	spec.Parameters = params

	secrets, d := decodeSecrets(file, node.GetMap("secrets"))
	diags = append(diags, d...)
	spec.Secrets = secrets

	images, d := decodeImages(file, node.GetMap("images"), spec.Mode)
	diags = append(diags, d...)
	spec.Images = images

	builds, d := decodePackageBuilds(file, node.GetMap("builds"))
	diags = append(diags, d...)
	spec.Builds = builds

	resources, d := decodeResources(file, node.GetMap("resources"))
	diags = append(diags, d...)
	spec.Resources = resources

	networks, d := decodeNetworks(file, node.GetMap("networks"))
	diags = append(diags, d...)
	spec.Networks = networks

	volumes, d := decodeVolumes(file, node.GetMap("volumes"))
	diags = append(diags, d...)
	spec.Volumes = volumes

	services, d := decodeServices(file, node.GetMap("services"))
	diags = append(diags, d...)
	spec.Services = services

	tasks, d := decodeTasks(file, node.GetMap("tasks"))
	diags = append(diags, d...)
	spec.Tasks = tasks

	dbs, d := decodeDatabases(file, node.GetMap("databases"))
	diags = append(diags, d...)
	spec.Databases = dbs
	for id, db := range spec.Databases {
		if _, ok := spec.Parameters[db.URLParameter]; ok || db.URLParameter == "" {
			continue
		}
		if spec.Parameters == nil {
			spec.Parameters = map[string]domain.Parameter{}
		}
		spec.Parameters[db.URLParameter] = domain.Parameter{
			Phase: domain.ParameterPhaseDeploy, Type: domain.ParameterTypeString,
			Description: "DSN for database " + id,
		}
	}

	pkg, d := decodePackage(file, node.GetMap("package"))
	diags = append(diags, d...)
	spec.Package = pkg

	xui, d := decodeXUI(file, node.GetMap("x-ui"))
	diags = append(diags, d...)
	spec.XUI = xui

	return spec, diags
}

func decodeParameters(file string, node *loader.Node) (map[string]domain.Parameter, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/parameters", "object", node)}
	}
	out := make(map[string]domain.Parameter, len(node.Map))
	seen := map[string]string{}
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/parameters/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("parameter id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		fold := strings.ToLower(id)
		if prev, ok := seen[fold]; ok {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("parameter id %q collides with %q ignoring case", id, prev)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		seen[fold] = id
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		p, d := decodeParameter(file, ptr, e.Value)
		diags = append(diags, d...)
		out[id] = p
	}
	return out, diags
}

func decodeParameter(file, ptr string, node *loader.Node) (domain.Parameter, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var p domain.Parameter
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"phase": {}, "type": {}, "default": {}, "minimum": {}, "maximum": {},
		"pattern": {}, "choices": {}, "description": {},
	})...)
	phase, d := requireString(file, node, "phase", ptr+"/phase")
	diags = append(diags, d...)
	if phase != "" && phase != "build" && phase != "deploy" {
		n := node.GetMap("phase")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			fmt.Sprintf("unsupported parameter phase %q", phase)), file, ptr+"/phase", n))
	}
	p.Phase = domain.ParameterPhase(phase)
	typ, d := requireString(file, node, "type", ptr+"/type")
	diags = append(diags, d...)
	switch typ {
	case "string", "integer", "number", "boolean", "enum":
		p.Type = domain.ParameterType(typ)
	case "":
	default:
		n := node.GetMap("type")
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			fmt.Sprintf("unsupported parameter type %q", typ)), file, ptr+"/type", n))
	}
	if def := node.GetMap("default"); def != nil {
		sc, d := decodeScalar(file, ptr+"/default", def)
		diags = append(diags, d...)
		p.Default = &sc
	}
	if mn := node.GetMap("minimum"); mn != nil {
		f, d := requireNumber(file, ptr+"/minimum", mn)
		diags = append(diags, d...)
		p.Minimum = &f
	}
	if mx := node.GetMap("maximum"); mx != nil {
		f, d := requireNumber(file, ptr+"/maximum", mx)
		diags = append(diags, d...)
		p.Maximum = &f
	}
	if pat := node.GetMap("pattern"); pat != nil {
		if pat.Kind != loader.KindString {
			diags = append(diags, typeErr(file, ptr+"/pattern", "string", pat))
		} else if len(pat.Str) > maxPatternBytes {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"parameter.pattern exceeds 1 KiB"), file, ptr+"/pattern", pat))
		} else if _, err := regexp.Compile(pat.Str); err != nil {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"parameter.pattern is not a valid RE2 expression"), file, ptr+"/pattern", pat))
		} else {
			p.Pattern = pat.Str
		}
	}
	if ch := node.GetMap("choices"); ch != nil {
		if ch.Kind != loader.KindSeq {
			diags = append(diags, typeErr(file, ptr+"/choices", "array", ch))
		} else if len(ch.Seq) < 1 || len(ch.Seq) > 100 {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"parameter.choices must contain 1–100 unique scalars"), file, ptr+"/choices", ch))
		} else {
			seen := map[string]struct{}{}
			for i, item := range ch.Seq {
				sc, d := decodeScalar(file, fmt.Sprintf("%s/choices/%d", ptr, i), item)
				diags = append(diags, d...)
				key := scalarKey(sc)
				if _, ok := seen[key]; ok {
					diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
						"parameter.choices must be unique"), file, fmt.Sprintf("%s/choices/%d", ptr, i), item))
				}
				seen[key] = struct{}{}
				p.Choices = append(p.Choices, sc)
			}
		}
	}
	if p.Type == domain.ParameterTypeEnum && len(p.Choices) == 0 {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaMissingField,
			"parameter type enum requires choices"), file, ptr, node))
	}
	if desc := node.GetMap("description"); desc != nil {
		if desc.Kind != loader.KindString {
			diags = append(diags, typeErr(file, ptr+"/description", "string", desc))
		} else if len(desc.Str) > maxDescriptionBytes {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"parameter.description exceeds 4 KiB"), file, ptr+"/description", desc))
		} else {
			p.Description = desc.Str
		}
	}
	return p, diags
}

func decodeSecrets(file string, node *loader.Node) (map[string]domain.Secret, []diagnostic.Diagnostic) {
	if node == nil {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/secrets", "object", node)}
	}
	out := make(map[string]domain.Secret, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/secrets/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("secret id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		// Reject any attempt to place a secret value.
		for _, fe := range e.Value.Map {
			if fe.Key == "value" || fe.Key == "values" || fe.Key == "data" {
				diags = append(diags, diagnostic.Error(diagnostic.CodeManifestSecretMisuse,
					"secret declarations must not contain values").
					WithSource(file, ptr+"/"+escape(fe.Key), fe.KeyLine, fe.KeyColumn).
					WithHint("Provide secrets at apply time via --secret-file"))
			}
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{
			"required": {}, "delivery": {}, "description": {},
		})...)
		sec := domain.Secret{Required: true, Delivery: "file"}
		if r := e.Value.GetMap("required"); r != nil {
			if r.Kind != loader.KindBool {
				diags = append(diags, typeErr(file, ptr+"/required", "boolean", r))
			} else {
				sec.Required = r.Bool
			}
		}
		if del := e.Value.GetMap("delivery"); del != nil {
			if del.Kind != loader.KindString {
				diags = append(diags, typeErr(file, ptr+"/delivery", "string", del))
			} else if del.Str != "file" {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					fmt.Sprintf("unsupported secret delivery %q", del.Str)), file, ptr+"/delivery", del).
					WithHint("Supported delivery: file"))
			} else {
				sec.Delivery = del.Str
			}
		}
		if desc := e.Value.GetMap("description"); desc != nil {
			if desc.Kind != loader.KindString {
				diags = append(diags, typeErr(file, ptr+"/description", "string", desc))
			} else if len(desc.Str) > maxDescriptionBytes {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
					"secret.description exceeds 4 KiB"), file, ptr+"/description", desc))
			} else {
				sec.Description = desc.Str
			}
		}
		out[id] = sec
	}
	return out, diags
}

func decodeImages(file string, node *loader.Node, mode domain.Mode) (map[string]domain.Image, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if node == nil {
		return nil, []diagnostic.Diagnostic{missing(file, "/spec/images", nil)}
	}
	if node.Kind != loader.KindMap {
		return nil, []diagnostic.Diagnostic{typeErr(file, "/spec/images", "object", node)}
	}
	if len(node.Map) == 0 {
		return nil, []diagnostic.Diagnostic{withLoc(diagnostic.Error(diagnostic.CodeSchemaMissingField,
			"spec.images must contain at least one image"), file, "/spec/images", node)}
	}
	out := make(map[string]domain.Image, len(node.Map))
	for _, e := range node.Map {
		id := e.Key
		ptr := "/spec/images/" + escape(id)
		if !identifierRe.MatchString(id) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
				fmt.Sprintf("image id %q is not a valid identifier", id)).
				WithSource(file, ptr, e.KeyLine, e.KeyColumn))
		}
		if e.Value == nil || e.Value.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr, "object", e.Value))
			continue
		}
		hasRef := e.Value.GetMap("ref") != nil
		hasBuild := e.Value.GetMap("build") != nil
		if hasRef == hasBuild {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"image must be either ref+delivery or build, not both or neither"), file, ptr, e.Value))
			continue
		}
		if hasBuild {
			diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{"build": {}})...)
			b, d := decodeImageBuild(file, ptr+"/build", e.Value.GetMap("build"))
			diags = append(diags, d...)
			if mode == domain.ModeRelease {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestBuildNotAllowed,
					fmt.Sprintf("image %q build is forbidden in release mode", id)), file, ptr+"/build", e.Value.GetMap("build")))
			}
			out[id] = domain.Image{Build: &b}
			continue
		}
		diags = append(diags, rejectUnknown(file, ptr, e.Value, map[string]struct{}{"ref": {}, "delivery": {}})...)
		delivery, d := requireString(file, e.Value, "delivery", ptr+"/delivery")
		diags = append(diags, d...)
		if delivery != "" && delivery != "pull" {
			n := e.Value.GetMap("delivery")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				fmt.Sprintf("unsupported image delivery %q", delivery)), file, ptr+"/delivery", n).
				WithHint("Supported delivery: pull"))
		}
		refNode := e.Value.GetMap("ref")
		if refNode == nil {
			diags = append(diags, missing(file, ptr+"/ref", e.Value))
			continue
		}
		img := domain.Image{Delivery: delivery}
		switch refNode.Kind {
		case loader.KindString:
			if refNode.Str == "" {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue, "ref must be a non-empty string"),
					file, ptr+"/ref", refNode))
				continue
			}
			img.Ref = refNode.Str
			if mode == domain.ModeRelease && !hasDigest(img.Ref) {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestMutableImageTag,
					fmt.Sprintf("image %q ref must be digest-pinned in release mode", id)),
					file, ptr+"/ref", refNode).WithHint("Use repository@sha256:<64-hex>"))
			}
		case loader.KindMap:
			pid, pd := decodeImageRefParameter(file, ptr+"/ref", refNode)
			diags = append(diags, pd...)
			img.RefParameter = pid
			if pid != "" && mode == domain.ModeRelease {
				diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeManifestMutableImageTag,
					fmt.Sprintf("image %q ref parameter %q is not digest-pinned in release mode", id, pid)),
					file, ptr+"/ref", refNode).WithHint("Set the parameter default to repository@sha256:<64-hex> and store that string in ref"))
			}
		default:
			diags = append(diags, typeErr(file, ptr+"/ref", "string or {parameter}", refNode))
			continue
		}
		out[id] = img
	}
	return out, diags
}

func decodeImageBuild(file, ptr string, node *loader.Node) (domain.ImageBuild, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var b domain.ImageBuild
	if node == nil || node.Kind != loader.KindMap {
		return b, []diagnostic.Diagnostic{typeErr(file, ptr, "object", node)}
	}
	diags = append(diags, rejectUnknown(file, ptr, node, map[string]struct{}{
		"context": {}, "dockerfile": {}, "include": {}, "exclude": {}, "args": {}, "expectedImageID": {},
	})...)
	ctx, d := requireString(file, node, "context", ptr+"/context")
	diags = append(diags, d...)
	if ctx != "" {
		wp, err := path.ParseWorkspacePath(ctx)
		if err != nil {
			n := node.GetMap("context")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
				file, ptr+"/context", n))
		} else {
			b.Context = wp
		}
	}
	df, d := requireString(file, node, "dockerfile", ptr+"/dockerfile")
	diags = append(diags, d...)
	if df != "" {
		wp, err := path.ParseWorkspacePath(df)
		if err != nil {
			n := node.GetMap("dockerfile")
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidPath, err.Error()),
				file, ptr+"/dockerfile", n))
		} else {
			b.Dockerfile = wp
		}
	}
	inc := node.GetMap("include")
	if inc == nil {
		diags = append(diags, missing(file, ptr+"/include", node))
	} else if inc.Kind != loader.KindSeq || len(inc.Seq) == 0 {
		diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
			"build.include must be a non-empty array"), file, ptr+"/include", inc))
	} else {
		for i, item := range inc.Seq {
			if item == nil || item.Kind != loader.KindString || item.Str == "" {
				diags = append(diags, typeErr(file, fmt.Sprintf("%s/include/%d", ptr, i), "non-empty string", item))
				continue
			}
			b.Include = append(b.Include, item.Str)
		}
	}
	if excl := node.GetMap("exclude"); excl != nil {
		if excl.Kind != loader.KindSeq {
			diags = append(diags, typeErr(file, ptr+"/exclude", "array", excl))
		} else {
			for i, item := range excl.Seq {
				if item == nil || item.Kind != loader.KindString || item.Str == "" {
					diags = append(diags, typeErr(file, fmt.Sprintf("%s/exclude/%d", ptr, i), "non-empty string", item))
					continue
				}
				b.Exclude = append(b.Exclude, item.Str)
			}
		}
	}
	if args := node.GetMap("args"); args != nil {
		if args.Kind != loader.KindMap {
			diags = append(diags, typeErr(file, ptr+"/args", "object", args))
		} else {
			b.Args = map[string]domain.ValueSource{}
			for _, ae := range args.Map {
				if !buildArgRe.MatchString(ae.Key) {
					diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaInvalidIdentifier,
						fmt.Sprintf("invalid build arg name %q", ae.Key)).
						WithSource(file, ptr+"/args/"+escape(ae.Key), ae.KeyLine, ae.KeyColumn))
				}
				vs, d := decodeValueSource(file, ptr+"/args/"+escape(ae.Key), ae.Value)
				diags = append(diags, d...)
				b.Args[ae.Key] = vs
			}
		}
	}
	if eid := node.GetMap("expectedImageID"); eid != nil {
		if eid.Kind != loader.KindString || !sha256IDRe.MatchString(eid.Str) {
			diags = append(diags, withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue,
				"expectedImageID must be sha256:<64-lowercase-hex>"), file, ptr+"/expectedImageID", eid))
		} else {
			b.ExpectedImageID = eid.Str
		}
	}
	return b, diags
}
