# Quickstart (clean-room)

Use only this document, `examples/`, and a source tree. Do **not** pipe remote
scripts into a shell (`curl | sh` is forbidden).

## Prerequisites

- Go 1.27.1 (see `go.mod`)
- Make
- Optional: Docker Engine + Compose plugin (for live apply)

## Build from source

```bash
make check
make build
./bin/iacpack version
```

Verify the binary you will run (`sha256sum ./bin/iacpack`) when using a release
candidate from `dist/rehearsal/` — compare against `SHA256SUMS`.

## Author → validate → lock → build

```bash
mkdir -p /tmp/iacpack-demo && cd /tmp/iacpack-demo
cp -a /path/to/iacpack/examples/minimal/. .
/path/to/iacpack/bin/iacpack validate --file iacpack.yaml
/path/to/iacpack/bin/iacpack lock --file iacpack.yaml
/path/to/iacpack/bin/iacpack validate --file iacpack.yaml --locked
/path/to/iacpack/bin/iacpack build --file iacpack.yaml --format directory --output /tmp/iacpack-out
```

Release example (digest-pinned images):

```bash
cp -a /path/to/iacpack/examples/release/. .
# … same lock/validate/build flow
```

## Plan / apply (Docker or Podman)

Requires a Compose-compatible engine. Default: Podman when available, else
Docker (`--container-engine` / `IACPACK_CONTAINER_ENGINE`). Secrets are
**files**, never flags containing values:

```bash
mkdir -p /tmp/secrets && printf 'changeme\n' > /tmp/secrets/db-password && chmod 600 /tmp/secrets/db-password
./bin/iacpack plan /tmp/out.iacbundle --secrets-dir /tmp/secrets
./bin/iacpack apply /tmp/out.iacbundle --secrets-dir /tmp/secrets
./bin/iacpack status --instance default
./bin/iacpack destroy --instance default   # volumes retained by default
```

One id can still be passed as `--secret-file db-password=/tmp/db.secret`; that pair overrides the same name from `--secrets-dir`. Before any state write, apply lists registry images it will pull and stops with exit 6 when a local image, host path, secret file, or referenced deploy parameter is missing. Host preparation: `scripts/prepare-apply-host.sh`.

Apply starts each service with `--no-deps --force-recreate`, waits for health
via engine `ps`/`inspect` (not `compose ps --status`), and continues after a
per-service start/ready failure. Writable `:z` binds render as
`read_only: false`; a still-RO mount is named in `APPLY.ACTION`. YAML 1.1 bool
command args are quoted (`"yes"`, `"on"`). Contract:
[runtime-apply-v1.md](../reference/runtime-apply-v1.md).

## Authoring UI

```bash
scripts/launch.sh
# menu → 1) GUI — then pick workspace; open http://127.0.0.1:7432/

./bin/iacpack serve --workspace /path/to/project
# open http://127.0.0.1:7432/ — loopback only
```

## Import existing Compose (draft)

If the workspace already has Compose (and optionally Dockerfiles):

```bash
./bin/iacpack import propose --workspace /path/to/project \
  --compose compose.yaml --compose stack9.yml --output json
```

Discover also lists `compose.*`, `docker-compose.*`, and `stack*.yml|yaml` (depth ≤ 3).

Or in the UI: **Import draft…** → multi-select discover hits and/or type paths →
**Propose** → review refusals, parameter gaps, and secret declaration stubs →
optionally set non-secret parameter defaults / toggle stubs → **Accept draft**
(applies stubs in memory; optional `.env.example` placeholders) → **Save**.

Propose never writes disk (except the optional explicit `.env.example` write on
Accept). Accept only replaces the in-memory Project.

Import fidelity is defined in [ADR-020](../adr/020-compose-import-fidelity-layers.md),
[ADR-021](../adr/021-deploy-gpu-reservations.md), and the
[capability matrix](../reference/compose-import-fidelity.md). What landed after
1.1.0, including the wizard, uploads, and `scripts/run-product.sh`, is in
[IMPORT_AUTHORING.md](IMPORT_AUTHORING.md).
Console walkthrough: [CONSOLE.md](CONSOLE.md). Important
differences from plain `docker compose`:

- Propose does **not** read project `.env`, `--env-file`, or shell env; only
  interpolation **defaults written in the compose file** (`${VAR:-default}`)
  may resolve to literals or parameter defaults.
- Partial drafts are normal: policy-denied services (for example service-level
  `devices:` / GPU CDI) are excluded with **warnings**; Accept is blocked only
  when `propose.ok` is false (errors on included services or empty draft).
- Deploy GPU is `resources.gpu` (`1..8`, `all`, or `devices-only`) (ADR-021).
  `gpu: all` without CDI `nvidia.com/gpu=all` stops apply before containers
  start (`APPLY.GPU_UNAVAILABLE`). `gpu: devices-only` plus `ldLibraryPath`
  bind-mounts `/dev/nvidia*` and sets `NVIDIA_VISIBLE_DEVICES` to the mounted
  GPU indexes (`0` for `/dev/nvidia0`). On enforcing SELinux that service
  also gets `security_opt: label=disable`, because `container_t` cannot open
  `/dev/nvidia*`. `void` hides those devices from CUDA.
- Short volume mode `:z` / `:Z` stays writable; `readOnly: true` only with
  `:ro`. String `command: start` becomes argv. An empty argv element stays
  `{literal: ""}`. `${SECRET}` in command or env becomes a secret reference
  or `parts`, not a dropped token. `user:` is kept (`name` or `uid:gid`).
  Render quotes YAML 1.1 bool argv (`yes`/`no`/`on`/`off`) and emits
  `read_only: false` on writable binds.
- `spec.builds` is compiled by `iacpack build` (pnpm/npm/yarn/bun, frozen
  lockfile). Apply mounts the payload; it does not run the package manager.
- Empty typed deploy params (`integer`/`boolean`) must not be `""`; empty
  optional strings are omitted from Compose env.
- When Compose omits `user`, propose reads local image `Config.User`. A numeric
  uid is `service.user`. A name is `IMPORT.USER_NAME` and is not written. A
  missing image uses the last Dockerfile `USER`, or warns `IMPORT.USER_ABSENT`.
  Re-import keeps a user already stored in the draft.
- Services that share a pull ref are linked with `environmentFrom`. Only keys
  whose values differ stay on the child (`IMPORT.ENV_FROM`). `plan` then omits
  `PLAN.ENV_MISSING` for that pair.
- `spec.databases` adds a database on an existing server. Compile emits task
  `db-<id>` after `healthy` and rewrites `urlParameter` to a DSN. The password
  stays a secret reference. A SQL file whose SHA-256 changed runs again and
  must tolerate a second execution. Compose import still does not invent
  migration tasks (`IMPORT.MIGRATE_GAP`).
- Tag-only image refs import as `mode: development`. Release requires
  `repository@sha256:<64-hex>`. After manifest or policy edits, run `iacpack lock`
  again. `plan` and `apply` stop with `lock устарел` when the workspace draft
  is newer than the lock inside the bundle, and print `iacpack diff --from`.
  `iacpack bundle` writes a portable kit and `project.iacbundle`. Empty host
  paths and unembedded local images are warnings. `plan` and `apply` of the kit
  directory are the gate, then they start that archive. Full operator steps:
  [GUIDE.md](GUIDE.md).

Unsupported or policy-denied fields (`privileged`, absolute host binds, Docker
socket, service `devices`) are omitted with diagnostics/refusals
(ADR-016/017/020). Absolute host binds are warnings unless `--import-strict`;
the service stays. `env_file` / secret-like keys / Compose secrets become
**declaration stubs** only — values are never read (ADR-018). Repeated full
services are first-wins. `!override` / `!reset` replaces that field on the
same service id, including a fragment that has no image.

Or generate reviewable GitHub helpers (no auto-publish):

```bash
./bin/iacpack release init-templates --workspace /path/to/project --dry-run --output json
./bin/iacpack release init-templates --workspace /path/to/project
```

UI: **Release templates…** → Preview → Confirm write. Writes
`.github/workflows/iacpack-ci.yml`, gitignore/gitattributes markers, and
`docs/iacpack-release.md`. Overwrite requires `--force` / Force checkbox.
Never pushes, never calls `gh`, never publishes (ADR-019).

## Clean-room checklist

- [ ] No ambient `.env` auto-load required
- [ ] No network during validate/build
- [ ] Docs never recommend curl-pipe-shell
- [ ] Secret values only via files / Docker secret stores
