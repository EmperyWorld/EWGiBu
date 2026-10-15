# Requirements traceability — v1.0 / v1.1

Maps each numbered acceptance criterion from `TZ.md` §§23–24 to evidence.
Status: `automated` | `manual` | `pending` | `n/a-platform`.
Unverified rows must not be marked complete.

**Product version:** `1.1.0` (TZ §5.1–5.3 feature scope; phases P00–P19).
Remaining operator/CI items below are release gates, not missing features.

## v1.0 (TZ §23)

| # | Criterion (summary) | Evidence | Status |
|---|---------------------|----------|--------|
| 1 | YAML/JSON same canonical + artifact hash | `internal/manifest/loader`, `internal/manifest/v1alpha1.TestYAMLJSONEquivalence`, `internal/adapters/cli/p06` | automated |
| 2 | Duplicate/unknown/aliases/unsupported → stable diagnostics | negative corpus `testdata/manifests/negative`, `TestNegativeCorpus` | automated |
| 3 | Path traversal / abs / links / collisions rejected | `internal/domain/path`, `internal/inputs`, `internal/bundle.TestVerifyRejects*` | automated |
| 4 | Two builds byte-identical across hosts | `scripts/p15/reproduce-build.sh` (Linux local OK); CI job `reproduce-multi-os` (ubuntu/macOS/windows) | automated (CI matrix) |
| 5 | Declared input changes digest; undeclared ignored | `internal/inputs`, lock/build tests | automated |
| 6 | validate/build no network/subprocess/workspace mutate | archrule + validate/build tests; `internal/archrule` | automated |
| 7 | Secret canary absent | `TestSecretCanary*`, plan/build/apply canary tests | automated |
| 8 | Bundle no undeclared/unsafe entries | `internal/bundle` verify suite + fuzz | automated |
| 9 | Release images by digest; lock drift fails | lock/policy tests; release examples | automated |
| 10 | Compose `docker compose config --quiet` | `scripts/p15/docker-e2e.sh` (Docker or Podman docker-CLI stand-in) | automated (2026-09-29, engine=podman) |
| 11 | Cycle diagnostic + deterministic graph | `internal/compile` graph tests + golden | automated |
| 12 | Two-service fixture install + health | apply fake-provider unit; live via `IACPACK_DOCKER_APPLY=1 make docker-e2e` (`testdata/e2e/engine-apply`) | automated (2026-09-29, engine=podman apply/status/destroy) |
| 13 | Re-apply no duplicates | `internal/app/apply` noop/idempotency tests | automated |
| 14 | Failure per stage → exit/journal/recovery | apply failure tests + `docs/release/RECOVERY.md` | automated |
| 15 | Interrupted → completed/pending/unknown | resume/state tests | automated |
| 16 | Cleanup preserves pre-existing/volumes | destroy/apply compensation tests | automated |
| 17 | Destroy retains volumes by default | `internal/app/destroy` | automated |
| 18 | CLI JSON schemas + stdout/stderr | `schemas/cli/v1` + CLI golden validation helpers | automated |
| 19 | Performance §19.2 | `FULL=1 scripts/p15/perf-bench.sh` → p95 validate 6 ms / 200 svc+10k files; `PERFORMANCE.md` | measured (target met) |
| 20 | Core coverage + security/fuzz/race | `scripts/p15/coverage-core.sh`, fuzz smoke, `make check` | automated |
| 21 | Release checksums/signatures/SBOM/provenance | `make test-release` / `scripts/release-rehearsal.sh` | automated |
| 22 | Thin installer ShellCheck | `scripts/p15/shellcheck.sh` | automated |
| 23 | Offline bundle without network | OCI layout + images tests; offline install doc | automated |
| 24 | Docs init→build→apply without hidden steps | `docs/release/QUICKSTART.md` clean-room checklist | manual verified |
| 25 | No open blocker/critical security findings | `docs/release/THREAT_MODEL.md` reconfirm 2026-09-29 | manual |

## v1.1 Web UI (TZ §24)

| # | Criterion | Evidence | Status |
|---|-----------|----------|--------|
| 1 | Two-service without YAML | `web/e2e/two-service.spec.ts` | automated |
| 2 | CLI/API equivalent diagnostics + preview | `p11` + httpapi tests | automated |
| 3 | Import→edit→export semantics | serialize/commands tests + e2e save | automated |
| 4 | Positions-only ≠ infra output | ADR-012 + graphMap/semantic hash tests | automated |
| 5 | External edit without reload | `web/e2e/sync.spec.ts` | automated |
| 6 | Stale save conflict | httpapi ETag tests + sync UI | automated |
| 7 | Invalid external keeps last valid | sync.spec invalidDisk | automated |
| 8 | No read outside workspace | httpapi confinement + Save As tests | automated |
| 9 | Browser never gets secrets | API redaction + canary tests | automated |
| 10 | WCAG 2.2 AA | `web/e2e/a11y.spec.ts` + checklist sign-off below | automated + manual |

### WCAG manual sign-off (v1.1.0)

Date: **2026-09-29**. Automated suite green (`pnpm test:e2e` a11y). Checklist items
1–10 in `docs/reference/ui-a11y-checklist.md` reviewed against current UI; residual
React Flow chrome gaps remain accepted (documented AT alternatives). **Signed for
release readiness** pending human publisher confirmation at tag time.

## Gap policy

- **n/a-platform:** Neither Docker nor Podman usable — script exits 0 with SKIP.
  Podman closes this gate only as a docker-compatible CLI/socket stand-in; it is
  not a first-class v1 provider (`SUPPORT_MATRIX.md`).
- **CI matrix:** `reproduce-multi-os` must be green on GitHub Actions before
  treating §23 #4 as fully closed across OS.
- Human **signed publish** remains `PUBLISH.md` only — never agent push.
