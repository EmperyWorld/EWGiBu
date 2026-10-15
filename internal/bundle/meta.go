// Package bundle implements deterministic .iacbundle packing and hostile-input verification.
package bundle

import (
	"iacpack/internal/buildinfo"
	"iacpack/internal/domain/ir"
	"iacpack/internal/policy"
)

// FormatVersion is the bundle archive contract.
const FormatVersion = "iacpack.bundle/v1"

// Default quotas (TZ.md §13.2).
const (
	DefaultMaxEntries       = 50000
	DefaultMaxEntryBytes    = 4 * 1024 * 1024 * 1024
	DefaultMaxExpandedBytes = 20 * 1024 * 1024 * 1024
	DefaultMaxRatio         = 200
	DefaultMaxPathBytes     = 255
	HardMaxEntries          = 100000
	HardMaxExpandedBytes    = 100 * 1024 * 1024 * 1024
)

// Meta is the bundle.json document (does not hash itself).
type Meta struct {
	FormatVersion string           `json:"formatVersion"`
	Project       MetaProject      `json:"project"`
	Compiler      MetaCompiler     `json:"compiler"`
	Provider      MetaProvider     `json:"provider"`
	Policy        MetaDigest       `json:"policy"`
	Schema        MetaDigest       `json:"schema"`
	Runtime       MetaRuntime      `json:"runtime"`
	Capabilities  MetaCapabilities `json:"capabilities"`
	Entries       []MetaEntry      `json:"entries"`
}

// MetaProject identifies the packaged project.
type MetaProject struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Mode         string `json:"mode"`
	SemanticHash string `json:"semanticHash"`
}

// MetaCompiler records tool identity.
type MetaCompiler struct {
	Product string `json:"product"`
	Version string `json:"version"`
}

// MetaProvider records provider profile.
type MetaProvider struct {
	Profile string `json:"profile"`
	Version string `json:"version"`
}

// MetaDigest is an identified digest.
type MetaDigest struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	SHA256 string `json:"sha256"`
}

// MetaRuntime declares required runtime.
type MetaRuntime struct {
	ComposePluginMin string `json:"composePluginMin"`
}

// MetaCapabilities records bundle capabilities.
type MetaCapabilities struct {
	EmbedImages    bool `json:"embedImages"`
	IncludeRuntime bool `json:"includeRuntime"`
	HasPreview     bool `json:"hasPreview"`
}

// MetaEntry describes one archive member other than bundle.json.
type MetaEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

// NewMeta builds bundle.json content for a deployment and entry list.
func NewMeta(d ir.Deployment, pol policy.CompilePolicy, schemaSHA string, polSHA string, entries []MetaEntry, hasPreview bool) Meta {
	return Meta{
		FormatVersion: FormatVersion,
		Project: MetaProject{
			Name: d.Project.Name, Version: d.Project.Version,
			Mode: d.Mode, SemanticHash: d.SemanticHash,
		},
		Compiler: MetaCompiler{Product: buildinfo.Product, Version: buildinfo.Version},
		Provider: MetaProvider{Profile: d.Profile, Version: "1"},
		Policy:   MetaDigest{Name: pol.Name, SHA256: polSHA},
		Schema:   MetaDigest{ID: "iacpack.schemas.manifest.v1alpha1", SHA256: schemaSHA},
		Runtime:  MetaRuntime{ComposePluginMin: "2.24"},
		Capabilities: MetaCapabilities{
			EmbedImages: d.Package.EmbedImages, IncludeRuntime: d.Package.IncludeRuntime, HasPreview: hasPreview,
		},
		Entries: entries,
	}
}
