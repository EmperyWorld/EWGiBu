// Package lockfile defines the canonical iacpack.lock.json document (RFC 8785).
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/buildinfo"
	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/inputs"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/oci"
	"iacpack/internal/params"
	"iacpack/internal/policy"
	"iacpack/internal/schemaembed"
)

// FormatVersion is the lockfile format identifier.
const FormatVersion = "iacpack.lock/v1"

// SchemaID is the checked-in lock schema $id.
const SchemaID = "iacpack.schemas.lock.v1"

// Diagnostic codes for lock/drift.
const (
	CodeLockParse           = "LOCK.PARSE_ERROR"
	CodeLockDrift           = "LOCK.DRIFT"
	CodeLockImageUnresolved = "LOCK.IMAGE_UNRESOLVED"
	CodeLockImageResolve    = "LOCK.IMAGE_RESOLVE"
)

// Document is the lockfile content model.
type Document struct {
	FormatVersion   string               `json:"formatVersion"`
	Manifest        ManifestLock         `json:"manifest"`
	Schema          DigestRef            `json:"schema"`
	Provider        ProviderLock         `json:"provider"`
	Compiler        CompilerLock         `json:"compiler"`
	Mode            string               `json:"mode"`
	Policy          PolicyLock           `json:"policy"`
	BuildParameters map[string]any       `json:"buildParameters"`
	Inputs          []InputLock          `json:"inputs"`
	Images          map[string]ImageLock `json:"images"`
}

// ManifestLock records manifest digests.
type ManifestLock struct {
	SHA256       string `json:"sha256"`
	SemanticHash string `json:"semanticHash"`
}

// DigestRef is an identified artifact digest.
type DigestRef struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// ProviderLock records provider identity.
type ProviderLock struct {
	Profile string `json:"profile"`
	Version string `json:"version"`
}

// CompilerLock records tool identity (no host/time).
type CompilerLock struct {
	Product string `json:"product"`
	Version string `json:"version"`
}

// PolicyLock records effective compile policy digest.
type PolicyLock struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// InputLock is one declared input file.
type InputLock struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

// ImageLock is a resolved image.
type ImageLock struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest,omitempty"`
	Build  bool   `json:"build,omitempty"`
}

// BuildOptions configures lock document construction.
type BuildOptions struct {
	ManifestBytes []byte
	Project       domain.Project
	Policy        policy.CompilePolicy
	BuildParams   map[string]params.Resolved
	Inputs        []inputs.FileRecord
	UpdateImages  bool
	Resolver      oci.Resolver
}

// Build constructs a canonical lock Document.
func Build(opts BuildOptions) (Document, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	semHash, err := v1alpha1.SemanticHash(opts.Project)
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, "semantic hash: "+err.Error()))
		return Document{}, diags
	}
	manSum := sha256.Sum256(opts.ManifestBytes)
	schemaSum := sha256.Sum256(schemaembed.ManifestV1Alpha1())
	polBytes, err := policy.CanonicalBytes(opts.Policy)
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeInternal, "policy canonical: "+err.Error()))
		return Document{}, diags
	}
	polSum := sha256.Sum256(polBytes)

	imgs, idiags := resolveImages(opts.Project, opts.UpdateImages, opts.Resolver)
	diags = append(diags, idiags...)

	inLocks := make([]InputLock, 0, len(opts.Inputs))
	for _, r := range opts.Inputs {
		inLocks = append(inLocks, InputLock{
			Path: r.Path, Size: r.Size, Mode: r.Mode, SHA256: r.SHA256, Source: r.Source,
		})
	}
	sort.SliceStable(inLocks, func(i, j int) bool { return inLocks[i].Path < inLocks[j].Path })

	doc := Document{
		FormatVersion: FormatVersion,
		Manifest: ManifestLock{
			SHA256:       hex.EncodeToString(manSum[:]),
			SemanticHash: semHash,
		},
		Schema: DigestRef{
			ID:     "iacpack.schemas.manifest.v1alpha1",
			SHA256: hex.EncodeToString(schemaSum[:]),
		},
		Provider: ProviderLock{
			Profile: string(opts.Project.Spec.Target.Profile),
			Version: "1",
		},
		Compiler: CompilerLock{
			Product: buildinfo.Product,
			Version: buildinfo.Version,
		},
		Mode:            string(opts.Project.Spec.Mode),
		Policy:          PolicyLock{Name: opts.Policy.Name, SHA256: hex.EncodeToString(polSum[:])},
		BuildParameters: params.PublicMap(opts.BuildParams),
		Inputs:          inLocks,
		Images:          imgs,
	}
	if doc.BuildParameters == nil {
		doc.BuildParameters = map[string]any{}
	}
	if doc.Images == nil {
		doc.Images = map[string]ImageLock{}
	}
	return doc, diags
}

func resolveImages(project domain.Project, update bool, resolver oci.Resolver) (map[string]ImageLock, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if resolver == nil {
		resolver = oci.DeniedResolver{}
	}
	out := map[string]ImageLock{}
	ids := make([]string, 0, len(project.Spec.Images))
	for id := range project.Spec.Images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		img := project.Spec.Images[id]
		if img.Build != nil {
			out[id] = ImageLock{Build: true, Ref: "build:" + id}
			continue
		}
		if strings.TrimSpace(img.Ref) == "" && img.RefParameter != "" {
			if project.Spec.Mode == domain.ModeDevelopment {
				out[id] = ImageLock{Ref: ""}
				diags = append(diags, diagnostic.Warning(CodeLockImageUnresolved,
					fmt.Sprintf("image %q ref is parameter %q with no default; development preview has no digest", id, img.RefParameter)).
					WithSource("", "/spec/images/"+id+"/ref", 0, 0).
					WithHint("Set the parameter default to a pull ref, then lock again"))
				continue
			}
			diags = append(diags, diagnostic.Error(CodeLockImageUnresolved,
				fmt.Sprintf("image %q ref is parameter %q with no default; release lock requires a digest-pinned ref", id, img.RefParameter)).
				WithSource("", "/spec/images/"+id+"/ref", 0, 0))
			continue
		}
		ref := img.Ref
		if d, ok := oci.DigestOf(ref); ok {
			out[id] = ImageLock{Ref: ref, Digest: d}
			continue
		}
		if !update {
			if project.Spec.Mode == domain.ModeDevelopment {
				out[id] = ImageLock{Ref: ref}
				diags = append(diags, diagnostic.Warning(CodeLockImageUnresolved,
					fmt.Sprintf("image %q ref %q is a tag; development preview uses it without a digest", id, ref)).
					WithSource("", "/spec/images/"+id+"/ref", 0, 0).
					WithHint("Release mode still requires repository@sha256:<64-hex>. lock --update-images pins when a resolver is configured"))
				continue
			}
			diags = append(diags, diagnostic.Error(CodeLockImageUnresolved,
				fmt.Sprintf("image %q ref %q is not digest-pinned; re-run with --update-images to resolve", id, ref)).
				WithSource("", "/spec/images/"+id+"/ref", 0, 0).
				WithHint("Release mode requires digest-pinned refs; development tags need explicit --update-images"))
			continue
		}
		pinned, digest, err := resolver.Resolve(ref)
		if err != nil {
			// Never include credentials from error text beyond safe message.
			diags = append(diags, diagnostic.Error(CodeLockImageResolve,
				fmt.Sprintf("failed to resolve image %q: %s", id, redactErr(err))).
				WithSource("", "/spec/images/"+id+"/ref", 0, 0))
			continue
		}
		out[id] = ImageLock{Ref: pinned, Digest: digest}
	}
	return out, diags
}

func redactErr(err error) string {
	s := err.Error()
	lower := strings.ToLower(s)
	for _, needle := range []string{"bearer ", "basic ", "password=", "token="} {
		if i := strings.Index(lower, needle); i >= 0 {
			return s[:i] + needle + "<redacted>"
		}
	}
	return s
}

// MarshalCanonical returns RFC 8785 bytes plus trailing newline for file writes.
func MarshalCanonical(doc Document) ([]byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	canon, err := canonicaljson.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(canon, '\n'), nil
}

// Parse loads a lock document from JSON bytes.
func Parse(data []byte) (Document, error) {
	var doc Document
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("%s: %w", CodeLockParse, err)
	}
	if doc.FormatVersion != FormatVersion {
		return Document{}, fmt.Errorf("%s: unsupported formatVersion %q", CodeLockParse, doc.FormatVersion)
	}
	return doc, nil
}

// CompareDrift returns diagnostics when current differs from locked.
func CompareDrift(locked Document, current Document) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	lb, _ := MarshalCanonical(locked)
	cb, _ := MarshalCanonical(current)
	if string(lb) != string(cb) {
		// Produce precise field-level hints.
		if locked.Manifest.SHA256 != current.Manifest.SHA256 {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "manifest bytes drifted from lockfile"))
		}
		if locked.Manifest.SemanticHash != current.Manifest.SemanticHash {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "manifest semantic hash drifted from lockfile"))
		}
		if locked.Policy.SHA256 != current.Policy.SHA256 {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "compile policy digest drifted from lockfile"))
		}
		if locked.Schema.SHA256 != current.Schema.SHA256 {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "schema digest drifted from lockfile"))
		}
		if !mapsEqual(locked.BuildParameters, current.BuildParameters) {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "build parameters drifted from lockfile"))
		}
		if !inputsEqual(locked.Inputs, current.Inputs) {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "declared inputs drifted from lockfile"))
		}
		if !imagesEqual(locked.Images, current.Images) {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "image digests drifted from lockfile"))
		}
		if len(diags) == 0 {
			diags = append(diags, diagnostic.Error(CodeLockDrift, "lockfile content drifted"))
		}
	}
	diagnostic.SortDeterministic(diags)
	return diags
}

func mapsEqual(a, b map[string]any) bool {
	ab, _ := canonicaljson.Marshal(a)
	bb, _ := canonicaljson.Marshal(b)
	return string(ab) == string(bb)
}

func inputsEqual(a, b []InputLock) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func imagesEqual(a, b map[string]ImageLock) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
