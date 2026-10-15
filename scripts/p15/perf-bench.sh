#!/bin/sh
# Indicative performance bench (TZ §19.2 methodology).
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT"
OUT="$ROOT/dist/rc-evidence/perf"
FIX="$ROOT/testdata/perf/gen"
rm -rf "$OUT" "$FIX"
mkdir -p "$OUT" "$FIX/files"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
make -s build

N_SERVICES="${N_SERVICES:-50}"
N_FILES="${N_FILES:-500}"
if [ "${FULL:-0}" = "1" ]; then
  N_SERVICES=200
  N_FILES=10000
fi

i=1
while [ "$i" -le "$N_FILES" ]; do
  head -c 1024 /dev/zero >"$FIX/files/f-$(printf '%05d' "$i").bin"
  i=$((i + 1))
done

{
  printf '%s\n' \
    'apiVersion: iacpack.io/v1alpha1' \
    'kind: Project' \
    'metadata:' \
    '  name: perf' \
    '  version: 0.1.0' \
    'spec:' \
    '  target:' \
    '    profile: compose-docker/v1' \
    '  mode: release' \
    '  package:' \
    '    include:' \
    '      - files/**' \
    '  images:' \
    '    app:' \
    '      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001' \
    '      delivery: pull' \
    '  services:'
  s=1
  while [ "$s" -le "$N_SERVICES" ]; do
    printf '    svc%03d:\n      image: app\n' "$s"
    s=$((s + 1))
  done
} >"$FIX/iacpack.yaml"

BIN="$ROOT/bin/iacpack"
w=1
while [ "$w" -le 5 ]; do
  "$BIN" validate --file "$FIX/iacpack.yaml" >/dev/null 2>&1 || true
  w=$((w + 1))
done

: >"$OUT/validate-times-ms.txt"
m=1
while [ "$m" -le 30 ]; do
  start=$(date +%s%N)
  "$BIN" validate --file "$FIX/iacpack.yaml" >/dev/null 2>&1 || true
  end=$(date +%s%N)
  echo $(((end - start) / 1000000)) >>"$OUT/validate-times-ms.txt"
  m=$((m + 1))
done

# Compute p95 with awk (no python dependency).
HOST=$(uname -s 2>/dev/null || echo unknown)
ARCH=$(uname -m 2>/dev/null || echo unknown)
NCPU=$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null || echo 0)
MEM_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
awk -v ns="$N_SERVICES" -v nf="$N_FILES" -v host="$HOST" -v arch="$ARCH" -v ncpu="$NCPU" -v memkb="$MEM_KB" '
{
  a[NR]=$1
}
END {
  n=NR
  for (i=1;i<=n;i++) for (j=i+1;j<=n;j++) if (a[i]>a[j]) { t=a[i]; a[i]=a[j]; a[j]=t }
  idx=int(0.95*(n-1))+1
  if (idx<1) idx=1
  if (idx>n) idx=n
  p50=a[int((n+1)/2)]
  p95=a[idx]
  status=(ns<200)?"indicative":"measured"
  printf "{\n"
  printf "  \"metric\": \"validate_wall_ms\",\n"
  printf "  \"n_services\": %d,\n", ns
  printf "  \"n_files\": %d,\n", nf
  printf "  \"samples\": %d,\n", n
  printf "  \"p50_ms\": %d,\n", p50
  printf "  \"p95_ms\": %d,\n", p95
  printf "  \"max_ms\": %d,\n", a[n]
  printf "  \"host\": \"%s/%s\",\n", host, arch
  printf "  \"ncpu\": %d,\n", ncpu+0
  printf "  \"mem_kb\": %d,\n", memkb+0
  printf "  \"runner\": \"local\",\n"
  printf "  \"reference_min\": \"4vCPU/8GiB\",\n"
  printf "  \"status\": \"%s\"\n", status
  printf "}\n"
}' "$OUT/validate-times-ms.txt" | tee "$OUT/validate.json"

echo "perf-bench done → $OUT"
