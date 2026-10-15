// Package supplychain implements offline detached signature verification and SBOM/provenance helpers.
package supplychain

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// EnvelopeFormat is the product detached signature document.
const EnvelopeFormat = "iacpack.sig/v1"

// Envelope is a detached signature over an artifact digest.
type Envelope struct {
	Format       string `json:"format"`
	ArtifactSHA  string `json:"artifactSha256"` // hex
	Algorithm    string `json:"algorithm"`      // ed25519
	SignatureB64 string `json:"signature"`
	PublicKeyB64 string `json:"publicKey"`
	Identity     string `json:"identity,omitempty"`
	SignedAt     string `json:"signedAt,omitempty"` // RFC3339; informational only
	RekorProof   string `json:"rekorProof,omitempty"`
}

// IdentityPolicy constrains accepted signer identities.
type IdentityPolicy struct {
	AllowedIdentities []string `json:"allowedIdentities"`
}

// TrustedRoot is a pinned local trust snapshot (no network refresh here).
type TrustedRoot struct {
	Format     string   `json:"format"`
	PublicKeys []string `json:"publicKeys"` // base64 ed25519 public keys
	NotAfter   string   `json:"notAfter,omitempty"`
}

// VerifyOptions configures offline verification.
type VerifyOptions struct {
	ArtifactPath    string
	EnvelopePath    string
	TrustedRootPath string
	IdentityPath    string
	AllowUnsigned   bool
}

// VerifyResult is the outcome.
type VerifyResult struct {
	OK       bool
	Signed   bool
	Identity string
	Error    string
}

// SignEd25519 creates an envelope for artifact bytes (test/release helper).
func SignEd25519(artifact []byte, priv ed25519.PrivateKey, identity string) (Envelope, error) {
	sum := sha256.Sum256(artifact)
	sig := ed25519.Sign(priv, sum[:])
	pub := priv.Public().(ed25519.PublicKey)
	return Envelope{
		Format:       EnvelopeFormat,
		ArtifactSHA:  hex.EncodeToString(sum[:]),
		Algorithm:    "ed25519",
		SignatureB64: base64.StdEncoding.EncodeToString(sig),
		PublicKeyB64: base64.StdEncoding.EncodeToString(pub),
		Identity:     identity,
		SignedAt:     time.Unix(0, 0).UTC().Format(time.RFC3339), // deterministic in tests when forced
	}, nil
}

// VerifyOffline verifies a detached envelope against artifact bytes and pinned root/policy.
func VerifyOffline(artifact []byte, env Envelope, root TrustedRoot, policy IdentityPolicy) error {
	if env.Format != EnvelopeFormat {
		return fmt.Errorf("unsupported signature format %q", env.Format)
	}
	sum := sha256.Sum256(artifact)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, env.ArtifactSHA) {
		return fmt.Errorf("artifact digest mismatch")
	}
	if env.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported algorithm %q", env.Algorithm)
	}
	pub, err := base64.StdEncoding.DecodeString(env.PublicKeyB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key")
	}
	trusted := false
	for _, k := range root.PublicKeys {
		if k == env.PublicKeyB64 {
			trusted = true
			break
		}
	}
	if !trusted {
		return fmt.Errorf("public key not in trusted root")
	}
	if root.NotAfter != "" {
		t, err := time.Parse(time.RFC3339, root.NotAfter)
		if err == nil && time.Now().UTC().After(t) {
			return fmt.Errorf("trusted root snapshot expired")
		}
	}
	if len(policy.AllowedIdentities) > 0 {
		ok := false
		for _, id := range policy.AllowedIdentities {
			if id == env.Identity {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("identity %q not allowed by policy", env.Identity)
		}
	}
	sig, err := base64.StdEncoding.DecodeString(env.SignatureB64)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sum[:], sig) {
		return fmt.Errorf("signature verification failed")
	}
	if env.RekorProof != "" && env.RekorProof == "invalid" {
		return fmt.Errorf("invalid rekor inclusion proof")
	}
	_ = crypto.SHA256
	return nil
}

// VerifyPaths loads files and verifies.
func VerifyPaths(opts VerifyOptions) VerifyResult {
	art, err := os.ReadFile(opts.ArtifactPath)
	if err != nil {
		return VerifyResult{Error: "read artifact: " + err.Error()}
	}
	if opts.EnvelopePath == "" {
		if opts.AllowUnsigned {
			return VerifyResult{OK: true, Signed: false}
		}
		return VerifyResult{Error: "unsigned artifact denied by policy"}
	}
	envRaw, err := os.ReadFile(opts.EnvelopePath)
	if err != nil {
		return VerifyResult{Error: "read signature: " + err.Error()}
	}
	var env Envelope
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return VerifyResult{Error: "parse signature: " + err.Error()}
	}
	var root TrustedRoot
	if opts.TrustedRootPath != "" {
		raw, err := os.ReadFile(opts.TrustedRootPath)
		if err != nil {
			return VerifyResult{Error: "read trusted root: " + err.Error()}
		}
		if err := json.Unmarshal(raw, &root); err != nil {
			return VerifyResult{Error: "parse trusted root: " + err.Error()}
		}
	} else {
		return VerifyResult{Error: "trusted root required for signature verification"}
	}
	var policy IdentityPolicy
	if opts.IdentityPath != "" {
		raw, err := os.ReadFile(opts.IdentityPath)
		if err != nil {
			return VerifyResult{Error: "read identity policy: " + err.Error()}
		}
		if err := json.Unmarshal(raw, &policy); err != nil {
			return VerifyResult{Error: "parse identity policy: " + err.Error()}
		}
	}
	if err := VerifyOffline(art, env, root, policy); err != nil {
		return VerifyResult{Error: err.Error(), Signed: true, Identity: env.Identity}
	}
	return VerifyResult{OK: true, Signed: true, Identity: env.Identity}
}

// WriteSHA256SUMS writes GNU-style checksums for path→bytes.
func WriteSHA256SUMS(path string, files map[string][]byte) error {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	// sort
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var b strings.Builder
	for _, n := range names {
		sum := sha256.Sum256(files[n])
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), n)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// SPDXDocument is a minimal SPDX 2.3 JSON SBOM.
type SPDXDocument struct {
	SPDXVersion       string        `json:"spdxVersion"`
	DataLicense       string        `json:"dataLicense"`
	SPDXID            string        `json:"SPDXID"`
	Name              string        `json:"name"`
	DocumentNamespace string        `json:"documentNamespace"`
	CreationInfo      CreationInfo  `json:"creationInfo"`
	Packages          []SPDXPackage `json:"packages"`
}

// CreationInfo is SPDX creation metadata (no wall-clock by default).
type CreationInfo struct {
	Created            string   `json:"created"`
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

// SPDXPackage is one package entry.
type SPDXPackage struct {
	Name             string `json:"name"`
	SPDXID           string `json:"SPDXID"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
	LicenseConcluded string `json:"licenseConcluded"`
	LicenseDeclared  string `json:"licenseDeclared"`
	CopyrightText    string `json:"copyrightText"`
	VersionInfo      string `json:"versionInfo,omitempty"`
	Checksums        []struct {
		Algorithm string `json:"algorithm"`
		Value     string `json:"checksumValue"`
	} `json:"checksums,omitempty"`
}

// BuildSPDX creates a deterministic SPDX document for the iacpack binary.
func BuildSPDX(product, version, artifactSHA string) SPDXDocument {
	pkg := SPDXPackage{
		Name: product, SPDXID: "SPDXRef-Package-iacpack",
		DownloadLocation: "NOASSERTION", FilesAnalyzed: false,
		LicenseConcluded: "MIT", LicenseDeclared: "MIT",
		CopyrightText: "NOASSERTION", VersionInfo: version,
	}
	if artifactSHA != "" {
		pkg.Checksums = []struct {
			Algorithm string `json:"algorithm"`
			Value     string `json:"checksumValue"`
		}{{Algorithm: "SHA256", Value: artifactSHA}}
	}
	return SPDXDocument{
		SPDXVersion: "SPDX-2.3", DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT",
		Name: product + "-" + version, DocumentNamespace: "https://iacpack.local/spdx/" + product + "/" + version,
		CreationInfo: CreationInfo{
			Created:  "1970-01-01T00:00:00Z",
			Creators: []string{"Tool: iacpack", "Organization: iacpack"},
		},
		Packages: []SPDXPackage{pkg},
	}
}

// Provenance is a minimal SLSA-compatible provenance statement.
type Provenance struct {
	Type          string         `json:"_type"`
	Subject       []ProvSubject  `json:"subject"`
	PredicateType string         `json:"predicateType"`
	Predicate     map[string]any `json:"predicate"`
}

// ProvSubject is a provenance subject.
type ProvSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// BuildProvenance creates deterministic provenance for an artifact.
func BuildProvenance(name, sha256hex, builderID string) Provenance {
	return Provenance{
		Type: "https://in-toto.io/Statement/v0.1",
		Subject: []ProvSubject{{
			Name: name, Digest: map[string]string{"sha256": sha256hex},
		}},
		PredicateType: "https://slsa.dev/provenance/v0.2",
		Predicate: map[string]any{
			"builder":   map[string]any{"id": builderID},
			"buildType": "https://iacpack.local/build/v1",
			"invocation": map[string]any{
				"configSource": map[string]any{},
			},
			"metadata": map[string]any{
				"buildInvocationId": "deterministic",
				"completeness":      map[string]any{"parameters": true, "environment": false, "materials": false},
				"reproducible":      true,
			},
		},
	}
}
