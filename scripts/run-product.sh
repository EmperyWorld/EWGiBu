#!/bin/sh
# Start the local iacpack authoring product, or validate the workspace manifest.
# Does not publish, does not read secret values, does not leave the workspace.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
BIN="$ROOT/bin/iacpack"
ADDR=${IACPACK_ADDR:-127.0.0.1:7432}
WS=${IACPACK_WORKSPACE:-$ROOT/testdata/ui/two-service}

cmd=${1:-serve}
if [ "$cmd" = "serve" ] || [ "$cmd" = "validate" ]; then
  if [ "${2:-}" != "" ]; then
    WS=$2
  fi
fi

if [ ! -x "$BIN" ]; then
  echo "building iacpack..." >&2
  (cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" make build)
fi

POLICY_ARGS=""
if [ -f "$WS/compile-policy.json" ]; then
  POLICY_ARGS="--policy $WS/compile-policy.json"
fi

case "$cmd" in
  serve)
    echo "authoring UI: http://$ADDR/  workspace: $WS" >&2
    # shellcheck disable=SC2086
    exec "$BIN" serve --workspace "$WS" --addr "$ADDR" $POLICY_ARGS
    ;;
  validate)
    # shellcheck disable=SC2086
    exec "$BIN" validate --file "$WS/iacpack.yaml" $POLICY_ARGS
    ;;
  *)
    echo "usage: scripts/run-product.sh [serve|validate] [workspace]" >&2
    echo "  serve     loopback authoring UI (default)" >&2
    echo "  validate  check iacpack.yaml in the workspace" >&2
    echo "Tag-based Compose imports use mode development so the graph can validate." >&2
    echo "Release mode needs every image ref as repository@sha256:<64-hex>." >&2
    exit 2
    ;;
esac
