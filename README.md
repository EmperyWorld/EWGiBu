# iacpack

`iacpack` is a source-distributed tool that helps package a multi-service product
into a portable, reviewable deployment bundle from an explicit declarative
manifest. It is infrastructure packaging tooling — not a hosted service, not a
remote access tool, and not a privilege escalation utility.

**License:** [MIT License](./LICENSE)
**Status:** stable **`1.1.0`** (TZ v1.0 + v1.1; phases P00–P19).
Release evidence: [`docs/release/TRACEABILITY.md`](./docs/release/TRACEABILITY.md),
[`docs/release/KNOWN_LIMITATIONS.md`](./docs/release/KNOWN_LIMITATIONS.md).
Changelog: [`docs/release/CHANGELOG.md`](./docs/release/CHANGELOG.md).
Human publish only: [`docs/release/PUBLISH.md`](./docs/release/PUBLISH.md).
Before a public signed tag on a Docker host: `make docker-e2e` (optionally
`IACPACK_DOCKER_APPLY=1`).

## Warranty and liability

This software is provided **AS IS**, without warranty of any kind. Authors and
contributors accept **no liability** for damages, data loss, security issues,
or operational outcomes. **You use it at your own risk** and remain responsible
for reviewing generated artifacts before applying them.

See also [`NOTICE`](./NOTICE).

## Source-only distribution

This repository contains **source code and contracts only**. Compiled binaries,
installer packages, and release artifacts are not committed and are not pushed
from this workspace. Build locally from source when you need a binary for
development or testing.

## Requirements

- Go **1.27.1** (pinned via `go.mod`)
- Make (optional, for documented targets)
- Network only if the Go toolchain itself must be downloaded the first time

Product module path is local (`iacpack`). Runnable source intentionally avoids
third-party/`host.example/...` imports so the built CLI does not embed foreign
module addresses.

## Quick start

Clean-room path: [`docs/release/QUICKSTART.md`](./docs/release/QUICKSTART.md).

```bash
# From a copy of this source tree:
make check
make build
./bin/iacpack version
./bin/iacpack init --output iacpack.yaml
./bin/iacpack validate --file iacpack.yaml
./bin/iacpack lock --file iacpack.yaml
./bin/iacpack validate --file iacpack.yaml --locked
./bin/iacpack build --file iacpack.yaml --format directory --output dist/project
./bin/iacpack inspect inputs --file iacpack.yaml
./bin/iacpack inspect graph --file iacpack.yaml --output mermaid
./bin/iacpack inspect manifest --file iacpack.yaml --output json
./bin/iacpack schema export
```

`make check` runs format verification, `go vet`, staticcheck, unit tests,
race tests, and govulncheck. Release rehearsal (no publish): `make test-release`.
Do **not** install via `curl | sh`.

## Repository map

| Path | Role |
|---|---|
| `TZ.md` | Authoritative product specification |
| `IMPLEMENTATION_PLAN.md` | Phased implementation plan for agents |
| `cmd/iacpack` | CLI entrypoint |
| `internal/` | Application, domain, and adapters |
| `schemas/` | Normative JSON Schema contracts |
| `policies/` | Immutable default compile policy |
| `examples/` | Valid Project manifests (minimal/release/development) |
| `docs/adr/` | Architecture Decision Records |
| `docs/reference/` | Human reference for implemented contracts |
| `docs/release/` | Traceability, threat model, RC / publish docs |
| `.cursor/rules/` | Persistent Cursor development rules |

## Current CLI surface (v1.1 RC)

| Command | Purpose |
|---|---|
| `iacpack init --output <path>` | Atomically create a minimal manifest (refuses overwrite) |
| `iacpack validate --file <path>` | Full schema/semantic/input/policy validation (read-only) |
| `iacpack lock --file <path>` | Write canonical `iacpack.lock.json` (atomic replace) |
| `iacpack build --format directory\|bundle --output <path>` | Deterministic artifact (new target only) |
| `iacpack verify / render / plan / apply / bundle / explain` | Bundle verify, compose render, portable kit, runtime ([GUIDE](./docs/release/GUIDE.md), [runtime-apply-v1](./docs/reference/runtime-apply-v1.md)) |
| `iacpack serve --workspace <dir>` | Loopback authoring API + embedded Web UI |
| `iacpack inspect …` / `schema export` / `version` | Read-only inspection and identity |
| `… --output json` | Machine envelope (`schemas/cli/v1/*.schema.json`) |

Operator guide: [`docs/release/GUIDE.md`](./docs/release/GUIDE.md).  
Full CLI table: [`docs/release/CLI.md`](./docs/release/CLI.md).  
OpenAPI contract: [`openapi/v1/openapi.yaml`](./openapi/v1/openapi.yaml).  
Generated TS client: [`web/src/api/generated/`](./web/src/api/generated/).

## Contributing and security

- Contribution expectations: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Vulnerability reporting: [`SECURITY.md`](./SECURITY.md)
- Design decisions: [`docs/adr/0000-index.md`](./docs/adr/0000-index.md)

## Remote publishing

Do not configure remotes or push unless a human explicitly requests it.
Local `git` history is optional for development; this bootstrap does not
require or perform a push.

> Public source snapshot: GitHub Actions workflow files are omitted.
