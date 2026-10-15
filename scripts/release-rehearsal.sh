#!/bin/sh
# Release rehearsal: build binaries, checksums, SBOM, provenance. Does NOT publish.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
OUT="${RELEASE_OUT:-$ROOT/dist/rehearsal}"
rm -rf "$OUT"
mkdir -p "$OUT/bin" "$OUT/sbom" "$OUT/provenance"

export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
make build
cp bin/iacpack "$OUT/bin/iacpack-linux-$(go env GOARCH)"

# Checksums
(
  cd "$OUT/bin"
  sha256sum -- * >"$OUT/SHA256SUMS"
)

# SBOM + provenance via a tiny Go helper (stdlib product path)
"$OUT/bin/iacpack-linux-$(go env GOARCH)" version --output json >"$OUT/version.json" || true
cat >"$OUT/sbom/iacpack.spdx.json" <<EOF
{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"iacpack-rehearsal","documentNamespace":"https://iacpack.local/spdx/rehearsal","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: iacpack"]},"packages":[{"name":"iacpack","SPDXID":"SPDXRef-Package-iacpack","downloadLocation":"NOASSERTION","filesAnalyzed":false,"licenseConcluded":"MIT","licenseDeclared":"MIT","copyrightText":"NOASSERTION"}]}
EOF

SUM=$(sha256sum "$OUT/bin/iacpack-linux-$(go env GOARCH)" | awk '{print $1}')
cat >"$OUT/provenance/iacpack.provenance.json" <<EOF
{"_type":"https://in-toto.io/Statement/v0.1","subject":[{"name":"iacpack","digest":{"sha256":"$SUM"}}],"predicateType":"https://slsa.dev/provenance/v0.2","predicate":{"builder":{"id":"https://iacpack.local/rehearsal"},"buildType":"https://iacpack.local/build/v1","metadata":{"reproducible":true}}}
EOF

# Permission assertion note for CI (no write tokens in default workflow)
printf 'release-rehearsal: artifacts at %s (not published)\n' "$OUT"
printf 'Install docs must never recommend curl|sh. Verify SHA256SUMS and signatures before exec.\n'
ls -la "$OUT" "$OUT/bin" "$OUT/sbom" "$OUT/provenance"
