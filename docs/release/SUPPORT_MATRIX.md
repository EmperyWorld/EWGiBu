# Provider / platform support matrix (v1.1)

| Capability | Support | Notes |
|------------|---------|-------|
| Provider profile `compose-docker/v1` | **Supported** | Sole v1 provider (ADR-007) |
| Podman | Out of scope (provider) | Post-v1 ADR; operator E2E / apply may use Podman as docker CLI/socket stand-in. Apply uses engine `ps`/`inspect` for ready (podman-compose 1.6 rejects `compose ps --status/--services`). See [runtime-apply-v1.md](../reference/runtime-apply-v1.md). |
| Kubernetes / k3s | Out of scope | Post-v1 gate |
| CLI Linux amd64 | Supported | Primary CI |
| CLI Linux arm64 | Supported | Cross-build in release job |
| CLI macOS / Windows | Supported build targets | Runtime Apply still expects Docker-compatible Compose on host |
| Loopback Web UI | Supported | `127.0.0.1` / `::1` only |
| Remote/hosted UI | Out of scope | |
| Offline OCI layout embed | Supported | ADR-013 |
| Host hooks / plugins | Forbidden | ADR-009 |

## Remaining for public v1/v1.1 (not new providers)

After P19, close release gates in `TRACEABILITY.md` / `PUBLISH.md` (container
engine E2E, cross-OS builders, formal perf, human sign-off). Do not expand this
matrix with a first-class Podman/k8s/Windows installer without a post-v1 ADR.

## Docker Compose

Generated Compose must pass `docker compose config --quiet` (or Podman
compose-compatible equivalent) when an engine is available. Render quotes
YAML 1.1 bool argv and empty `{literal: ""}` arguments, emits explicit
`read_only: false` on writable binds, and runs lifecycle tasks with
`--profile iacpack-task`. A task `entrypoint` replaces the image ENTRYPOINT;
`user` is a uid, `uid:gid`, or name. File secrets are `../secrets/<id>` beside the
rendered compose file. CI/`docker-e2e` skips when neither Docker nor Podman
is usable. Package-manager frontend builds (`spec.builds`) run on the build
machine; apply only mounts the payload.
