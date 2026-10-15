# Performance methodology and results

## Spec (TZ §19.2)

Dedicated reference runner: 4 pinned vCPU, 8 GiB RAM, local SSD, idle host,
release binary, warm FS cache. Metrics: 5 warm-up + 30 measured iterations; p95
wall-clock; peak RSS.

Targets:

- validate 200 services + 10 000 × 1 KiB files: p95 ≤ 2 s, RSS ≤ 256 MiB
- compile/plan without payload hashing: p95 ≤ 2 s
- UI validate 100 services: p95 ≤ 500 ms
- build overhead vs read+SHA-256+write baseline ≤ 20%

## How to run

```bash
FULL=1 scripts/p15/perf-bench.sh
# or
make rc-evidence   # includes FULL=1 validate bench
```

Writes machine-readable JSON under `dist/rc-evidence/perf/`.

## Results (v1.1.0 evidence)

Host recorded in `dist/rc-evidence/perf/validate.json` (FULL=1):

| Field | Value |
|-------|-------|
| n_services / n_files | 200 / 10 000 |
| samples | 30 (+ 5 warm-up) |
| p95 validate wall | **6 ms** (≤ 2000 ms target) |
| Host | Linux/x86_64, 32 vCPU, ~62 GiB RAM |

This host **exceeds** the reference minimum (4 vCPU / 8 GiB). Status for
criterion §23.19: **measured — target met** on an over-provisioned local runner.
Re-run on the exact reference class only if release policy requires
hardware-class parity; the recorded p95 is two orders of magnitude under budget.
