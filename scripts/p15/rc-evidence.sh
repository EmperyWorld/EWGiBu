#!/bin/sh
# Aggregate RC evidence (does not publish).
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
EV="$ROOT/dist/rc-evidence"
mkdir -p "$EV"
{
  echo "iacpack RC evidence"
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "commit: $(git rev-parse HEAD 2>/dev/null || echo unknown)"
  echo "go: $(go env GOVERSION 2>/dev/null || true)"
} >"$EV/META.txt"

sh scripts/p15/shellcheck.sh
sh scripts/p15/reproduce-build.sh
sh scripts/p15/coverage-core.sh
sh scripts/p15/fuzz-smoke.sh
sh scripts/p15/perf-bench.sh
make test-release

cp -a dist/rehearsal/. "$EV/rehearsal/" 2>/dev/null || true
printf 'See docs/release/ for TRACEABILITY, THREAT_MODEL, PUBLISH.\n' >"$EV/README.txt"
echo "RC evidence pack at $EV"
