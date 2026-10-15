#!/bin/sh
# Double-build the CLI and compare SHA-256 digests.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
OUT="$ROOT/dist/rc-evidence/repro"
rm -rf "$OUT"
mkdir -p "$OUT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
export CGO_ENABLED=0

go build -trimpath -ldflags "-s -w" -o "$OUT/iacpack.a" ./cmd/iacpack
go build -trimpath -ldflags "-s -w" -o "$OUT/iacpack.b" ./cmd/iacpack

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "neither sha256sum nor shasum found" >&2
    exit 1
  fi
}

suma=$(sha256_file "$OUT/iacpack.a")
sumb=$(sha256_file "$OUT/iacpack.b")
printf '%s  iacpack.a\n%s  iacpack.b\n' "$suma" "$sumb" >"$OUT/SHA256SUMS"
if [ "$suma" != "$sumb" ]; then
  echo "reproducibility FAIL: digests differ" >&2
  exit 1
fi
echo "reproducibility OK: $suma"
echo "$suma" >"$OUT/DIGEST"
