# Manifest reference — iacpack.io/v1alpha1

Normative machine contract: [`schemas/manifest/v1alpha1/schema.json`](../../schemas/manifest/v1alpha1/schema.json)  
(`$id`: `iacpack.schemas.manifest.v1alpha1`). Semantic rules: `TZ.md` §§9–10, ADR-012, ADR-015.  
Compose import mapping (inverse emit): [ADR-020](../adr/020-compose-import-fidelity-layers.md), [compose-import-fidelity.md](compose-import-fidelity.md).

## Root

| Field | Required | Notes |
|---|---:|---|
| `apiVersion` | yes | `iacpack.io/v1alpha1` |
| `kind` | yes | `Project` |
| `metadata.name` | yes | `^[a-z][a-z0-9-]{0,62}$` |
| `metadata.version` | yes | SemVer without leading `v` |
| `metadata.description` | no | ≤ 4 KiB; non-secret |
| `spec` | yes | see below |

## Spec

| Field | Required | Notes |
|---|---:|---|
| `target.profile` | yes | `compose-docker/v1` |
| `mode` | yes | `development` \| `release` |
| `parameters` | no | build/deploy typed values |
| `secrets` | no | declarations only; `delivery: file` |
| `images` | yes | `ref`+`delivery: pull` **or** `build` (dev-only) |
| `builds` | no | package-manager output produced by `iacpack build` |
| `resources` | no | workspace includes |
| `networks` | no | managed `{}` or `{external:{name}}` |
| `volumes` | no | `{retention}` default `retain`, or external |
| `services` | yes | ≥1, ≤500 |
| `tasks` | no | ≤500 lifecycle one-shots |
| `package` | no | packaging defaults |
| `x-ui` | no | UI-only; excluded from semantic hash |

## Discriminated value sources

`{literal}`, `{parameter: <id>}`, or `{secret: <id>}`. Env values may also be
`{parts: [...]}` of those three (a URL whose password is a secret). No raw
`${VAR}` and no ambient env. A secret arm is an id, never a value.

## Images

- **Pull:** `ref`, `delivery: pull`. Release requires `@sha256:<64-hex>`.
- **Build (development only):** `context`, `dockerfile`, `include`, optional `exclude`/`args`/`expectedImageID`.

## Package builds

`spec.builds.<id>`: `context` (directory with `package.json`), `packageManager`
(`pnpm` | `npm` | `yarn` | `bun`), `install: frozen`, `script` (name from
`package.json`), `output`, optional `entry` (default `index.html`).

`iacpack build` runs the manager in a temporary copy of `context`. Missing
`package.json`, missing lockfile, unknown manager, empty output, or a missing
non-empty entry file is `BUILD.FRONTEND`. Apply only mounts
`./payload/build-<id>`. A service mount is `{build: <id>, target, readOnly}`.

## Databases

`spec.databases.<id>`: `on` (existing server service), `name`, `ownerFrom`
(service with `POSTGRES_USER` and a password secret), `migrations` (workspace
SQL directory), `urlParameter` (deploy parameter rewritten to a DSN). Compile
emits task `db-<id>` (`phase: migration`, after `healthy` on `on`). No second
Postgres container. The password stays a secret reference. Each SQL file is
recorded with its SHA-256. A changed file runs again and must tolerate a
second execution; an unchanged file is skipped.

## Services / tasks (summary)

Services: image, optional `user` (`name` or `uid:gid`), argv `command`/`entrypoint`.
An argv element is a non-empty string, `{literal: ""}` (empty argument, for
example `--logfile ""`), `{secret: <id>}`, or `{parts}` of strings, `{parameter}`,
and `{secret}`. Secret slots render as `${IACPACK_SECRET_…}`; the value is not
stored. Environment is `{literal}`, `{parameter}`, `{secret}`, or `{parts}` of
those. `environmentFrom: <service>` copies that service's environment first;
keys in `environment` override it. Import links same-image services that are
not a value-preserving env superset. Also: secret file mounts, ports, volume/resource/build mounts (`:z`/`:Z`
stay writable; `readOnly: true` only with `:ro`), networks, healthcheck (argv
only), restart allowlist, resource limits (`cpus`, `memory`, optional `gpu`
1..8 or `all` per ADR-021), `dependsOn`. Render quotes YAML 1.1 bool argv and
emits `read_only: false` on writable binds
([runtime-apply-v1.md](runtime-apply-v1.md)).

Tasks: `phase` `preStart`|`migration`|`postDeploy`, image, command, optional
`entrypoint` (replaces the image ENTRYPOINT; `[]` clears it) and `user`
(`uid`, `uid:gid`, or name), optional workdir/timeout/retry/`rerun`,
mounts/networks/secrets, deps with phase partial order (TZ §9.11). Rendered
with Compose profile `iacpack-task`. Every `rerun: oncePerDeployment` task
reruns when command, entrypoint, user, or mount file bytes change. Secret
bytes are not part of that hash.

## Package defaults

`generateReadme`/`generateEnvExample` default `true`; `includeRuntime`/`embedImages` default `false`. Conflicts with CLI format / compile policy yield `MANIFEST.PACKAGE_OPTION_CONFLICT`.

## x-ui

`nodes.<id>.{x,y,category?}`, optional `viewport.{x,y,zoom}` with zoom `0.1..4`. Unknown keys fail.

## Compile policy

Default: [`policies/default.compile.json`](../../policies/default.compile.json). Schema: [`schemas/policy/v1/schema.json`](../../schemas/policy/v1/schema.json).

`validate --policy <path>` overrides the embedded default. Evaluation enforces registry allowlists, capability gates (development builds, external networks/volumes, package options), and `resourceCeilings` (`POLICY.RESOURCE_CEILING` when a service exceeds cpus/memoryBytes).

## Examples

- [`examples/minimal/`](../../examples/minimal/) — minimal release
- [`examples/release/`](../../examples/release/) — full release shape
- [`examples/development/`](../../examples/development/) — build image + preStart task
