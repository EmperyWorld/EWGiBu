#!/bin/sh
# Operator container-engine E2E gate (TZ §23 #10 / #12).
# Accepts Docker Engine *or* Podman (docker-compatible CLI + local Unix socket).
# Exits 0 with SKIP when neither engine is usable (n/a-platform).
# Live apply/health is opt-in: IACPACK_DOCKER_APPLY=1
#   Uses testdata/e2e/engine-apply (real digests). Podman is a docker CLI
#   stand-in for this gate only — not a first-class v1 provider profile.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"

ENGINE=""
COMPOSE_CMD=""
SOCKET=""
SHIM_DIR=""
WS=""
APPLY_WS=""

cleanup() {
  if [ -n "${SHIM_DIR}" ] && [ -d "${SHIM_DIR}" ]; then
    rm -rf "${SHIM_DIR}"
  fi
  if [ -n "${WS}" ] && [ -d "${WS}" ]; then
    rm -rf "${WS}"
  fi
  if [ -n "${APPLY_WS}" ] && [ -d "${APPLY_WS}" ]; then
    rm -rf "${APPLY_WS}"
  fi
}
trap cleanup EXIT

resolve_engine() {
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    if podman compose version >/dev/null 2>&1; then
      ENGINE=podman
      COMPOSE_CMD="podman compose"
    elif command -v podman-compose >/dev/null 2>&1; then
      ENGINE=podman
      COMPOSE_CMD="podman-compose"
    else
      echo "docker-e2e: FAIL — podman found but no compose provider" >&2
      exit 1
    fi
    RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
    SOCKET="${IACPACK_DOCKER_SOCKET:-$RUNTIME_DIR/podman/podman.sock}"
    if [ ! -S "$SOCKET" ]; then
      systemctl --user start podman.socket >/dev/null 2>&1 || true
      i=0
      while [ "$i" -lt 20 ] && [ ! -S "$SOCKET" ]; do
        sleep 0.1
        i=$((i + 1))
      done
    fi
    if [ ! -S "$SOCKET" ]; then
      echo "docker-e2e: FAIL — podman socket missing at $SOCKET (start: systemctl --user start podman.socket)" >&2
      exit 1
    fi
    export IACPACK_CONTAINER_ENGINE=podman
    export IACPACK_DOCKER_SOCKET="$SOCKET"
    return 0
  fi
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    if docker compose version >/dev/null 2>&1; then
      ENGINE=docker
      COMPOSE_CMD="docker compose"
      SOCKET="${IACPACK_DOCKER_SOCKET:-/var/run/docker.sock}"
      export IACPACK_CONTAINER_ENGINE=docker
      export IACPACK_DOCKER_SOCKET="$SOCKET"
      return 0
    fi
  fi
  echo "docker-e2e: SKIP — neither podman nor docker usable (n/a-platform)" >&2
  exit 0
}

compose_config_quiet() {
  file=$1
  # Drop provider banner noise; keep failures.
  # shellcheck disable=SC2086
  if ! $COMPOSE_CMD -f "$file" config --quiet 2>/tmp/iacpack-compose-config.err; then
    cat /tmp/iacpack-compose-config.err >&2
    rm -f /tmp/iacpack-compose-config.err
    return 1
  fi
  rm -f /tmp/iacpack-compose-config.err
  return 0
}

resolve_engine
echo "docker-e2e: engine=$ENGINE compose='$COMPOSE_CMD' socket=$SOCKET"

export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
make -s build
BIN="$ROOT/bin/iacpack"

# --- §23 #10: compose config on release example ---
FIX="$ROOT/examples/release"
WS=$(mktemp -d)
cp -a "$FIX/." "$WS/"
MANIFEST="$WS/iacpack.yaml"
echo "docker-e2e: validate/lock/build (compose-config) in $WS"
"$BIN" validate --file "$MANIFEST"
"$BIN" lock --file "$MANIFEST"
OUT_BUNDLE="$WS/out-bundle"
OUT_DIR="$WS/out-dir"
"$BIN" build --file "$MANIFEST" --output "$OUT_BUNDLE" --format bundle
"$BIN" build --file "$MANIFEST" --output "$OUT_DIR" --format directory

COMPOSE=$(find "$OUT_DIR" "$OUT_BUNDLE" \( -name 'compose.preview.yaml' -o -name 'compose.yaml' -o -name 'compose*.yaml' \) -type f 2>/dev/null | sort | head -n 1 || true)
if [ -z "${COMPOSE}" ]; then
  echo "docker-e2e: FAIL — no compose yaml under build output" >&2
  exit 1
fi
echo "docker-e2e: compose config --quiet ($COMPOSE)"
compose_config_quiet "$COMPOSE"

BUNDLE=""
if [ -f "$OUT_BUNDLE" ]; then
  BUNDLE="$OUT_BUNDLE"
else
  BUNDLE=$(find "$OUT_BUNDLE" "$OUT_DIR" -name '*.iacbundle' -type f 2>/dev/null | sort | head -n 1 || true)
fi
if [ -z "${BUNDLE}" ] || [ ! -f "${BUNDLE}" ]; then
  echo "docker-e2e: FAIL — no bundle produced" >&2
  exit 1
fi
"$BIN" verify "${BUNDLE}"
SECRET_DIR="$WS/.secrets"
mkdir -p "$SECRET_DIR"
printf 'placeholder-not-a-real-secret\n' >"$SECRET_DIR/db-password"
chmod 0400 "$SECRET_DIR/db-password"
"$BIN" plan "${BUNDLE}" --secret-file "db-password=$SECRET_DIR/db-password" >/dev/null
echo "docker-e2e: compose-config gate OK"

# --- §23 #12: live apply (opt-in) on real-digest fixture ---
if [ "${IACPACK_DOCKER_APPLY:-0}" = "1" ]; then
  APPLY_FIX="$ROOT/testdata/e2e/engine-apply"
  APPLY_WS=$(mktemp -d)
  cp -a "$APPLY_FIX/." "$APPLY_WS/"
  echo "docker-e2e: apply smoke in $APPLY_WS (engine=$ENGINE)"
  "$BIN" validate --file "$APPLY_WS/iacpack.yaml"
  "$BIN" lock --file "$APPLY_WS/iacpack.yaml"
  APPLY_BUNDLE="$APPLY_WS/out-bundle"
  "$BIN" build --file "$APPLY_WS/iacpack.yaml" --output "$APPLY_BUNDLE" --format bundle
  "$BIN" verify "$APPLY_BUNDLE"
  "$BIN" plan "$APPLY_BUNDLE" >/dev/null
  "$BIN" apply "$APPLY_BUNDLE" --yes --docker-socket "$SOCKET"
  # status/destroy take project name (not workspace path); fixture metadata.name=engine-apply
  APPLY_PROJECT=engine-apply
  "$BIN" status "$APPLY_PROJECT"
  "$BIN" destroy "$APPLY_PROJECT" --yes --docker-socket "$SOCKET"
  # Ensure compose project containers are gone after real HostDestroy.
  if podman ps -a --format '{{.Names}}' 2>/dev/null | grep -F 'iacp-engine-apply' >/dev/null 2>&1; then
    echo "docker-e2e: FAIL — container still present after destroy" >&2
    podman ps -a --filter name=iacp-engine-apply >&2 || true
    exit 1
  fi
  if command -v docker >/dev/null 2>&1 && [ "$ENGINE" = docker ]; then
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -F 'iacp-engine-apply' >/dev/null 2>&1; then
      echo "docker-e2e: FAIL — container still present after destroy" >&2
      exit 1
    fi
  fi
  echo "docker-e2e: apply/destroy gate OK"
else
  echo "docker-e2e: SKIP live apply/health (set IACPACK_DOCKER_APPLY=1; uses testdata/e2e/engine-apply)"
fi

echo "docker-e2e: OK (engine=$ENGINE)"
