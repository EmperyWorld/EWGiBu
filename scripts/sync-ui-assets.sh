#!/bin/sh
# Sync web/dist into the Go embed tree deterministically (no mtime/path stamps).
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
SRC="$ROOT/web/dist"
DST="$ROOT/internal/adapters/httpapi/uistatic/dist"
if [ ! -f "$SRC/index.html" ]; then
  echo "missing $SRC/index.html; run: cd web && pnpm build" >&2
  exit 1
fi
rm -rf "$DST"
mkdir -p "$DST"
# Copy without preserving host timestamps into the embed tree.
cp -R "$SRC/." "$DST/"
# Normalize timestamps for reproducibility of the working tree presentation.
find "$DST" -type f -exec touch -t 202001010000 {} +
echo "synced UI assets -> $DST"
