package schemaembed

import (
	_ "embed"
)

//go:embed manifest_v1alpha1.json
var manifestV1Alpha1 []byte

//go:embed policy_v1.json
var policyV1 []byte

// ManifestV1Alpha1 returns the embedded manifest schema bytes.
func ManifestV1Alpha1() []byte {
	out := make([]byte, len(manifestV1Alpha1))
	copy(out, manifestV1Alpha1)
	return out
}

// PolicyV1 returns the embedded policy schema bytes.
func PolicyV1() []byte {
	out := make([]byte, len(policyV1))
	copy(out, policyV1)
	return out
}
