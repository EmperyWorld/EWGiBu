#!/bin/sh
# ShellCheck all packaging/release shell scripts.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
if ! command -v shellcheck >/dev/null 2>&1; then
  echo "shellcheck not installed — FAIL (required for P15)" >&2
  exit 1
fi
# Exclude SC1007 noise on intentional CDPATH='' assignments if any remain.
find scripts -type f -name '*.sh' -print0 | xargs -0 shellcheck -x -e SC1007
echo "shellcheck OK"
