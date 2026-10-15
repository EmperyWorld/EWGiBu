# Human publish procedure (only approved path)

**Do not publish from agent automation.** Default CI has `contents: read` only.

Product line ready for tag: **`1.1.0`** (see `TRACEABILITY.md`). Confirm CI
`reproduce-multi-os` before signing. Container E2E (§23 #10/#12) closed on the
evidence host with Podman as docker-CLI stand-in (2026-09-29).

1. Confirm all rows in `TRACEABILITY.md` are `automated` or explicitly signed
   off `manual`; no open blocker/critical in `THREAT_MODEL.md`.
2. Run on release hardware/CI: `make test-all`, confirm `reproduce-multi-os`
   green; re-run `IACPACK_DOCKER_APPLY=1 make docker-e2e` if the engine changes.
3. Create a **signed** annotated tag `v1.1.0` from the exact commit SHA under
   review.
4. Trigger the privileged release workflow (separate from PR CI) with minimal
   `contents: write` / signing identity — never on `pull_request`.
5. Attach binaries, `SHA256SUMS`, signatures, SBOM, provenance, schema archive,
   and `CHANGELOG.md` excerpt.
6. Verify download checksums and signatures before any install doc links go live.
7. Installation docs must never recommend `curl | sh`.

Local evidence: `make test-release` → `dist/rehearsal/`; perf → `dist/rc-evidence/perf/`.
