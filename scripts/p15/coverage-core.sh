#!/bin/sh
# Statement coverage floors for security-critical core packages.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
OUT="$ROOT/dist/rc-evidence/coverage"
mkdir -p "$OUT"
fail=0

check_pkg() {
  pkg="$1"
  min="$2"
  line=$(go test "$pkg" -cover -count=1 2>&1 | tee -a "$OUT/packages.txt" | tail -1)
  pct=$(printf '%s\n' "$line" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')
  if [ -z "$pct" ]; then
    echo "missing coverage for $pkg: $line" >&2
    fail=1
    return
  fi
  ok=no
  printf '%s %s\n' "$pct" "$min" | awk '{ if ($1+0 >= $2+0) exit 0; exit 1 }' && ok=yes
  if [ "$ok" != "yes" ]; then
    echo "$pkg coverage ${pct}% < ${min}%" >&2
    fail=1
    return
  fi
  echo "OK $pkg ${pct}% (min ${min}%)"
}

: >"$OUT/packages.txt"
check_pkg ./internal/domain/path 90
check_pkg ./internal/glob 85
check_pkg ./internal/canonicaljson 80
check_pkg ./internal/bundle 80
check_pkg ./internal/compile 80
check_pkg ./internal/manifest/v1alpha1 80
check_pkg ./internal/policy 60
check_pkg ./internal/inputs 65
check_pkg ./internal/lockfile 50

go test \
  ./internal/domain/path \
  ./internal/glob \
  ./internal/canonicaljson \
  ./internal/manifest/loader \
  ./internal/manifest/v1alpha1 \
  ./internal/bundle \
  ./internal/compile \
  ./internal/policy \
  ./internal/inputs \
  ./internal/lockfile \
  -coverprofile="$OUT/core.out" -count=1
go tool cover -func="$OUT/core.out" | tee "$OUT/core.txt"
tail -1 "$OUT/core.txt"

if [ "$fail" -ne 0 ]; then
  echo "coverage-core FAIL" >&2
  exit 1
fi
echo "coverage-core OK"
