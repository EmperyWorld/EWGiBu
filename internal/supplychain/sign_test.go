package supplychain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/supplychain"
)

func TestSignVerifyAndRejects(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	art := []byte("bundle-bytes")
	env, err := supplychain.SignEd25519(art, priv, "release@iacpack.local")
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	root := supplychain.TrustedRoot{Format: "iacpack.trusted-root/v1", PublicKeys: []string{pub}}
	pol := supplychain.IdentityPolicy{AllowedIdentities: []string{"release@iacpack.local"}}
	if err := supplychain.VerifyOffline(art, env, root, pol); err != nil {
		t.Fatal(err)
	}
	// tampered artifact
	if err := supplychain.VerifyOffline([]byte("other"), env, root, pol); err == nil {
		t.Fatal("tamper should fail")
	}
	// untrusted identity
	pol2 := supplychain.IdentityPolicy{AllowedIdentities: []string{"other@example"}}
	if err := supplychain.VerifyOffline(art, env, root, pol2); err == nil {
		t.Fatal("identity should fail")
	}
	// untrusted key
	root2 := supplychain.TrustedRoot{Format: "iacpack.trusted-root/v1", PublicKeys: []string{base64.StdEncoding.EncodeToString(make([]byte, 32))}}
	if err := supplychain.VerifyOffline(art, env, root2, pol); err == nil {
		t.Fatal("key should fail")
	}
	// invalid rekor proof marker
	env.RekorProof = "invalid"
	if err := supplychain.VerifyOffline(art, env, root, pol); err == nil {
		t.Fatal("rekor proof should fail")
	}
}

func TestVerifyPathsUnsignedPolicy(t *testing.T) {
	dir := t.TempDir()
	art := filepath.Join(dir, "a.iacbundle")
	if err := os.WriteFile(art, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := supplychain.VerifyPaths(supplychain.VerifyOptions{ArtifactPath: art, AllowUnsigned: true})
	if !r.OK || r.Signed {
		t.Fatalf("%+v", r)
	}
	r = supplychain.VerifyPaths(supplychain.VerifyOptions{ArtifactPath: art, AllowUnsigned: false})
	if r.OK {
		t.Fatal("unsigned denied")
	}
}

func TestSPDXAndProvenance(t *testing.T) {
	sum := sha256.Sum256([]byte("bin"))
	hexSum := hex.EncodeToString(sum[:])
	doc := supplychain.BuildSPDX("iacpack", "0.1.0-dev", hexSum)
	if len(doc.Packages) != 1 || doc.Packages[0].LicenseConcluded != "MIT" || doc.Packages[0].LicenseDeclared != "MIT" {
		t.Fatalf("release SBOM must declare the product MIT license: %+v", doc.Packages)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("invalid spdx json")
	}
	prov := supplychain.BuildProvenance("iacpack", hexSum, "https://iacpack.local/builder")
	if prov.Subject[0].Digest["sha256"] != hexSum {
		t.Fatal("digest")
	}
}
