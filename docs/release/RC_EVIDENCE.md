# Release candidate / stable evidence index

Local (non-published) pack for **1.1.0**:

| Path | Contents |
|------|----------|
| `dist/rc-evidence/rehearsal/` | Binaries, `SHA256SUMS`, SBOM, provenance (from `make test-release`) |
| `dist/rc-evidence/repro/` | Double-build digests |
| `dist/rc-evidence/coverage/` | Core coverage floors |
| `dist/rc-evidence/perf/` | FULL=1 validate bench JSON |

Commands:

```bash
make check
make test-integration test-e2e
make test-ui
make test-release reproduce-build coverage-core shellcheck
FULL=1 scripts/p15/perf-bench.sh
make docker-e2e   # Docker or Podman; SKIP if neither; IACPACK_DOCKER_APPLY=1 for live apply
# Evidence host 2026-09-29: IACPACK_DOCKER_APPLY=1 → engine=podman compose-config + apply/status/destroy OK
```

Supporting docs:

- [TRACEABILITY.md](./TRACEABILITY.md)
- [THREAT_MODEL.md](./THREAT_MODEL.md)
- [PERFORMANCE.md](./PERFORMANCE.md)
- [REPRODUCIBILITY.md](./REPRODUCIBILITY.md)
- [PUBLISH.md](./PUBLISH.md)
- [KNOWN_LIMITATIONS.md](./KNOWN_LIMITATIONS.md)

## Before public signed tag

1. Confirm CI `check` + `reproduce-multi-os` green.
2. Confirm `make docker-e2e` (and optionally `IACPACK_DOCKER_APPLY=1`) on Docker or Podman.
3. Human signed publish only — [PUBLISH.md](./PUBLISH.md).

Record SHA-256 of the Linux CLI from `SHA256SUMS` in the human release checklist
before tagging.
