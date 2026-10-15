#!/bin/sh
# Short fuzz smoke against known Fuzz* entrypoints.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
FUZZTIME="${FUZZTIME:-3s}"

run_fuzz() {
  pkg="$1"
  target="$2"
  echo "== $pkg $target =="
  go test "$pkg" -run=^$ -fuzz="$target" -fuzztime="$FUZZTIME"
}

run_fuzz ./internal/manifest/v1alpha1 FuzzDecode
run_fuzz ./internal/bundle FuzzVerify
run_fuzz ./internal/glob FuzzMatch
run_fuzz ./internal/inputs FuzzValidateRelPath
run_fuzz ./internal/canonicaljson FuzzMarshal
run_fuzz ./internal/domain/path FuzzParseWorkspacePath

echo "fuzz-smoke OK"
