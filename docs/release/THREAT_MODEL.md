# Threat model review (v1.1.0)

Scope: manifest → build → bundle → plan/apply → loopback UI/API → release CI.
**Reconfirm date:** 2026-09-29 (v1.1.0 readiness). Reconfirm again before each
public tag per `PUBLISH.md`.

## Assets

| Asset | Notes |
|-------|-------|
| Workspace source | Untrusted. `validate` does not execute it. `iacpack build` runs only a declared `spec.builds` package-manager script in a temp copy |
| Manifest / lock / bundle | Integrity + no secrets |
| Runtime state/journal | Instance identity; no secret values |
| Loopback session/CSRF | Authoring only |
| Release artifacts | Checksums, signatures, SBOM, provenance |
| Docker socket | Highest privilege boundary at apply time |

## Trust boundaries

1. Operator workstation ↔ CLI (user-controlled args/files).
2. CLI ↔ Docker engine (apply/plan). `validate` stays offline. `build` may run a declared package manager for `spec.builds` and may use the network for a frozen install.
3. Browser ↔ loopback API (same-origin, CSRF, workspace root fixed at serve).
4. CI ↔ release artifacts (least privilege; no publish from default workflow).

## Findings (blocker/critical)

| ID | Area | Finding | Severity | Resolution |
|----|------|---------|----------|------------|
| TM-01 | API/UI | Watcher event must not authorize write | — | Mitigated: If-Match + conflict UI (P14) |
| TM-02 | Secrets | Canary in outputs/logs | — | Mitigated: canary tests across plan/build/apply + import onboard |
| TM-03 | Paths | Traversal/symlink escape | — | Mitigated: typed paths + inputs + bundle verify |
| TM-04 | Archives | Zip/tar bombs, duplicates | — | Mitigated: quotas + reject tests + fuzz |
| TM-05 | OCI | Mutable tags in release | — | Mitigated: digest pin + lock drift |
| TM-06 | Docker | Privileged/socket mounts | — | Mitigated: default-deny operator/compile policy |
| TM-07 | Installer | curl\|sh encourage | — | Mitigated: docs forbid; quickstart uses make/build |
| TM-08 | CI | Over-broad write tokens | — | Mitigated: `permissions: contents: read` |
| TM-09 | Browser | XSS via manifest HTML | Medium residual | CSP + JSON/YAML APIs; no HTML render of manifest |
| TM-10 | Live engine E2E | No Docker/Podman on host | Gate SKIP / n/a-platform | `scripts/p15/docker-e2e.sh`; evidence 2026-09-29 used Podman stand-in |

**Blocker/critical open:** none.

## Residual risks (accepted for v1.1)

- Poll-based watcher debounce window.
- Canonical Save may drop YAML comments.
- React Flow third-party chrome a11y gaps (documented AT alternatives).
- Cross-OS reproduce relies on CI matrix `reproduce-multi-os` (must be green).
- Live apply/health needs Docker or Podman (`IACPACK_DOCKER_APPLY=1`); closed on
  evidence host 2026-09-29 with Podman docker-CLI stand-in.
