# Import and authoring work (unreleased)

Operator notes for the Compose import and local authoring changes after
**1.1.0**. Normative rules stay in [ADR-020](../adr/020-compose-import-fidelity-layers.md),
[ADR-021](../adr/021-deploy-gpu-reservations.md), and
[compose-import-fidelity.md](../reference/compose-import-fidelity.md).
This page records what was implemented and how to use it.

## Contracts

| Piece | Result |
| --- | --- |
| ADR-020 F0 | Fidelity matrix, QUICKSTART / KNOWN_LIMITATIONS, cursor rule |
| ADR-020 F1 | Image `repo:${TAG:-default}`, `mem_limit`, CPU normalize, healthcheck `CMD-SHELL`, bind `${VAR:-path}` |
| ADR-020 F2 | Partial `propose.ok`, `--profile` / `activeProfiles`, `--import-strict` |
| ADR-021 (F3) | `resources.gpu` is `1..8` or `all`. Render is NVIDIA `deploy.resources.reservations.devices` only |
| ADR-020 F5 | Healthcheck argv round-trips to `CMD-SHELL`. When Podman or Docker is present, `compose --profile debug config` is checked on a rendered sample |

Not done, on purpose: a separate Podman provider, Kubernetes, Windows, host hooks,
hosted UI, reading `.env` at propose, and translating CDI `devices:` into `resources.gpu`.

## Propose behavior

- **Mode.** A draft stays `release` only when every pull ref is already
  `repository@sha256:<64-hex>`. Any tag sets `spec.mode` to `development` and
  adds an info diagnostic. Release validation still rejects mutable tags.
- **Profiles.** Services with `profiles:` are skipped unless `--profile` /
  `activeProfiles` intersects them (`IMPORT.PROFILE_SKIPPED`, info). Included
  services keep `profiles` on the manifest and in rendered Compose.
- **Policy.** `devices`, `privileged`, host namespaces, and similar fields
  exclude that service with a **warning**. They do not fail `propose.ok` unless
  `--import-strict` / API `strict: true`. Accept follows `propose.ok`.
- **Secrets.** Secret-like env keys and `env_file` become declaration stubs.
  File contents and secret values are never read.
- **Binds.** Directory binds use `include: ["**"]` and `allowEmpty: true`, so an
  empty log directory is not `INPUT.EMPTY_MATCH`. If another resource binds a
  single file in the same `source`, that file is added to the directory
  resource `exclude` so the path is not packaged twice. Short mode `:z` / `:Z`
  stays writable; `readOnly: true` only when the mode contains `ro`. Render
  emits `read_only: false` for writable binds and quotes YAML 1.1 bool argv
  (`yes`/`no`/`on`/`off`).
- **Command.** String `command` / `entrypoint` (for example `command: start`)
  becomes argv words. Unbalanced quotes are omitted with a warning. An empty
  element `""` is `{literal: ""}`. `${SECRET}` in an argument becomes
  `{secret: id}` (or a `parts` element) and is not dropped. A URL such as
  `redis://:${PASSWORD}@host` becomes env `{parts}` of literals and a secret.
- **User.** `user:` is `service.user`. `${STACK_USER:-0:0}` imports the
  default `0:0` when it is a name or `uid:gid`. When Compose omits `user`,
  propose reads local `Config.User`: a numeric uid or `uid:gid` (including
  `0` and `0:0`) is written; a name is `IMPORT.USER_NAME` and is not written.
  A missing image uses the last Dockerfile `USER` the same way, or warns
  `IMPORT.USER_ABSENT`. An image with an empty `USER` and no Dockerfile `USER`
  leaves `service.user` unset. Re-import keeps a draft user
  (`--existing-project-file`, API `existingProject`, or Propose again in the
  UI). Operator steps: [GUIDE.md](GUIDE.md).
- **environmentFrom.** Services that share one pull ref are linked. The
  fuller env is the parent. The child keeps only keys whose values differ
  (`IMPORT.ENV_FROM`). A child that already contains the parent's values is
  left alone. Cycles stay forbidden. `plan` does not emit `PLAN.ENV_MISSING`
  for a pair that inherits.
- **`../` paths.** Paths that would leave the workspace are rebased to the
  workspace root with a warning. Files outside the workspace are not read.
- **GPU.** `deploy.resources.reservations.devices` with `driver` omitted or
  `nvidia`, `capabilities: [gpu]`, and `count` `1..8` or `all` becomes
  `resources.gpu`. `device_ids`, other drivers, and service-level `devices:`
  stay out of the draft. Podman apply behavior for missing CDI
  `nvidia.com/gpu=all`: [ADR-021](../adr/021-deploy-gpu-reservations.md),
  [runtime-apply-v1.md](../reference/runtime-apply-v1.md).
- **Second database.** Authors add `spec.databases` themselves. Compile emits
  task `db-<id>` after `healthy` on `on`, applies SQL by filename, and reruns
  a file when its SHA-256 changes. `urlParameter` becomes a DSN whose password
  is a secret reference. There is no second Postgres container. See
  [GUIDE.md](GUIDE.md) and [manifest-v1alpha1](../reference/manifest-v1alpha1.md).
- **SQL migrations.** Compose import does not create `spec.tasks`. Propose
  reports `IMPORT.MIGRATE_GAP` when it finds `**/migrations` with
  `baseline.sql` and/or `pending/`, and includes a YAML template. Greenfield:
  one product-owned `baseline.sql` + `pending/*.sql` + DB tracking table;
  `applied/` is history and is never replayed. Wire API services with
  `dependsOn` migrate `completedSuccessfully` (ADR-009). Example runner:
  [examples/pack-migrate](examples/pack-migrate). See
  [runtime-apply-v1.md](../reference/runtime-apply-v1.md#migration-tasks-sql-greenfield).

## Authoring UI

Import is a wizard: **Sources → Review → Folders → Secrets → Finish**.

- Sources accept a file, a folder, or a `.zip` / `.tar.gz` archive. Folder
  contents are written **into** the target directory (the picked folder name is
  not nested again).
- Each missing bind row can stay a stub, point at an existing workspace path
  (**Choose in workspace** lists one directory level; symlinks and `.env` names
  are skipped), or upload a file, folder, or archive.
- Folders **Prepare** writes only stub rows. Existing-path choices remap the
  draft first.
- Secrets: fill parameter defaults that Propose reported with no default.
  Secret checkboxes declare ids only.
- Finish shows a checklist. Accept is blocked while path warnings or empty
  parameter defaults remain, or while error diagnostics exist.
- The draft dialog uses the full window width. The post-Accept banner uses
  dark text on a light background.
- **Deploy in terminal** prints a script whose bundle output is
  `.iacpack-out/bundle.iacbundle` and whose `status` / `destroy` use
  `metadata.name`, not the bundle path.

`POST /api/v1/workspace/upload` accepts `file`, `tree`, `zip`, and `tar`.
That route allows a larger JSON body (about 96 MiB) than other API calls.
`GET /api/v1/workspace/files?dir=` lists one workspace directory.

## Images, lock, and preview

Development preview may use a tag and records `LOCK.IMAGE_UNRESOLVED` as a
**warning**. Release mode still requires a digest and fails the lock if the
ref is only a tag.

`iacpack build` and Compose preview compare the workspace `iacpack.lock.json`
with the current manifest (`--locked`). After the manifest, policy, or schema
changes, run `iacpack lock` again. A stale lock shows `LOCK.DRIFT` and preview
stays unavailable.

The workspace `testdata/ui/two-service` was pinned with digests that `skopeo inspect`
returned for the previous tags (chroma, elasticsearch, postgres, qdrant,
rabbitmq, redis, seaweedfs). Those refs are `repository@sha256:…` and `mode` is
`release`. Digests were not invented.

`lock --update-images` still does not call a registry by itself. The product
resolver remains closed unless a test injects one. Pin refs explicitly, as above.

## External networks

Default compile policy sets `externalNetworks: false`. An imported
`external: true` network then fails preview with `POLICY.CAPABILITY_DENIED`.

`testdata/ui/two-service/compile-policy.json` turns that capability on for this
workspace only. `scripts/run-product.sh` passes `--policy` when that file exists.

## Run

From the repository root:

```bash
scripts/run-product.sh
scripts/run-product.sh validate
scripts/run-product.sh serve /path/to/workspace
```

Default address is `http://127.0.0.1:7432/`. Default workspace is
`testdata/ui/two-service`. Override with `IACPACK_ADDR` and `IACPACK_WORKSPACE`.

After Accept, Save writes `iacpack.yaml`. Then:

```bash
iacpack lock --file iacpack.yaml --policy compile-policy.json
iacpack validate --file iacpack.yaml --policy compile-policy.json --locked
```

## Checks run while landing this work

Web: ESLint (zero warnings), `tsc` and TypeScript 7, OpenAPI client drift,
Vitest. Go: `gofmt`, `go vet` on the touched packages, and tests for import,
lock, validate, render, manifest, and HTTP. A full `make check` (race,
staticcheck, govulncheck, every package) and Playwright were not the gate for
this note.
