# Compose import fidelity matrix

Normative architecture: [ADR-020](../adr/020-compose-import-fidelity-layers.md).  
Product contract: `TZ.md` §9–§10. Baseline import: [ADR-016](../adr/016-compose-dockerfile-propose-import.md), [ADR-017](../adr/017-deeper-compose-import.md).

This document maps **official Compose** behavior to **iacpack import → manifest → render → runtime Compose**. It is not a promise of full [Compose Specification](https://docs.docker.com/reference/compose-file/) coverage.

## Reference layers (read order)

| Priority | Layer | Primary sources |
| --- | --- | --- |
| 1 | Compose Specification | [Services](https://docs.docker.com/reference/compose-file/services/), [Deploy](https://docs.docker.com/reference/compose-file/deploy/), [Interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/), [Profiles](https://docs.docker.com/compose/profiles/) |
| 2 | Runtime Compose | [docker compose](https://docs.docker.com/compose/); [podman compose](https://docs.podman.io/en/latest/markdown/podman-compose.1.html) (external provider) |
| 3 | iacpack contract | `TZ.md`, ADR-007/016/017/018/020, [manifest v1alpha1](manifest-v1alpha1.md), [KNOWN_LIMITATIONS](../release/KNOWN_LIMITATIONS.md) |
| 4 | Implementation | `internal/app/importpropose`, `internal/render/compose` |

When layers disagree, **security and TZ beat Compose**. Where ADR-020 promises inverse emit fidelity, implementation (layer 4) must converge to layers 1–3.

## Capability matrix

Legend:

- **Import** — `import propose` mapping into draft manifest
- **Emit** — `compose-docker/v1` render
- **Apply** — provider apply via rendered Compose
- **Severity** — diagnostic when not imported (target per ADR-020)

| Compose field / behavior (layer 1) | Official runtime notes (layer 2) | Import | Emit | Apply | Severity if omitted | ADR-020 phase |
| --- | --- | --- | --- | --- | --- | --- |
| `image` literal | `compose config` unchanged | Yes | Yes | Yes | — | — |
| `image: ${VAR:-full/ref}` | Resolved from `.env`/shell at runtime | Yes (peel nested defaults to a literal) | Yes | Yes | error if included service unresolved | F1 |
| `image: ${VAR}` / `${A:-${B}}` with no literal | Unresolved at runtime without a value | `{parameter: id}`, no default | When the parameter has a value | When resolved | info; draft stays valid | — |
| `image: repo:${TAG:-tag}` | Common pattern in docs | Yes | Yes | Yes | error if unresolved | — |
| `build` | Context relative to compose file dir | Yes | Yes (dev) | Yes | error if no image | — |
| `command` / `entrypoint` argv | Exec form | Yes | Yes | Yes | — | F1 (safe argv) |
| `command` / `entrypoint` string | Shell words (`command: start`) | Yes, argv words | Yes | Yes | warning if quotes are unbalanced | — |
| Argv empty string `""` | Empty argument (Redis `--logfile ""`) | `{literal: ""}` | `- ""` | Yes | — | — |
| Argv `yes`/`no`/`on`/`off` (YAML 1.1 bools) | Must stay strings in the container | Stored as strings | Quoted in YAML | Yes | — | — |
| `command` with `${PARAM}` | Runtime env | `{parameter}` or `{parts}` | Yes | Yes | info | F1 |
| `command` with `${SECRET}` | Password must stay its own argv slot | `{secret: id}` or a `parts` element; not dropped | `${IACPACK_SECRET_…}` | Yes | warning `SECRET_LIKE` | — |
| `environment` non-secret | Interpolation at runtime | Parameters / literals | Yes | Yes | info for new params | — |
| `environment` mixed literal + `${SECRET}` | URL with a password | `{parts: [literal, secret, literal]}` | Placeholder inside the string | Yes | info + `SECRET_LIKE` | — |
| `environment` secret-like keys | Often `.env` | `{secret: id}`; value not imported | Via secrets | Yes | warning `SECRET_LIKE` | F4 |
| `env_file` | Loaded by compose CLI | Never read; stub suggestions | No | No | warning | — |
| `ports` short/long | Interpolation | Yes | Yes | Yes | info for host params | — |
| `volumes` bind short/long | Relative to compose file | Yes (workspace paths) | Yes | Yes | warning path/rebase | F1 |
| Absolute host bind | Host path, not workspace-relative | Omitted | No | No | warning (error if `--import-strict`); service stays | — |
| `volumes` `${VAR:?}` required | Runtime | Parameter gap | When resolved | When resolved | warning/info | F1 |
| `networks` / external | External name | Yes | Yes | Yes | collision warning multi-file | — |
| `depends_on` | Conditions | Subset | Yes | Yes | warning unsupported conditions | — |
| `healthcheck` `CMD` | Exec probe | Yes | Yes | Yes | — | F1 |
| `healthcheck` `CMD-SHELL` | Shell script string | Yes (`/bin/sh`, `-c`, script) | `CMD-SHELL` | Yes | warning if omitted | — |
| `restart` | Allowlist subset | Yes | Yes | Yes | — | — |
| `deploy.resources.limits` | Supported by modern `docker compose` | Yes | Yes (`Deploy`) | Yes | warning if invalid cpu string | F1 |
| `mem_limit` | Same semantics as memory limit ([services](https://docs.docker.com/reference/compose-file/services/)) | Yes → `resources.memory` | Yes | Yes | warning if conflict/unresolved | — |
| `profiles` | `COMPOSE_PROFILES` / `--profile` | Skip/include via `activeProfiles`; included services store `profiles` | Yes | Provider `--profile` | info skip | **F2** |
| `deploy.resources.reservations.devices` GPU | [GPU how-to](https://docs.docker.com/compose/how-tos/gpu-support) | Yes → `resources.gpu` 1..8 or `all`, nvidia, `[gpu]` only (ADR-021) | Yes | Yes | warning if other driver or `device_ids` | **F3** |
| `devices:` (incl. CDI GPU) | Podman/CDI patterns | No; service excluded | No | No | **warning** (ADR-020 §7) | F2 |
| `privileged`, caps, host ns | Compose supports | Deny | Deny | Deny | error only if strict; else warning excluded | F2 |
| `user` | `name` or `uid:gid`; `${VAR:-0:0}` | Literal, or the compose default when it is a name or `uid:gid`. When Compose omits `user`, propose reads local image `Config.User`: numeric uid/`uid:gid` become `service.user`; a name is `IMPORT.USER_NAME` (not written). If the image is not local, the last `USER` instruction in the service Dockerfile is used the same way; otherwise `IMPORT.USER_ABSENT`. Re-import keeps an existing draft `user`. | Compose `user:` | Yes | warning if the Compose value is not a name or `uid:gid` | — |
| `container_name`, `logging`, `security_opt`, `ulimits` | Provider-specific | Omit | Omit | Omit | warning unsupported | — |
| Multi-file merge | Compose merge/include | First-wins, plus `!override` / `!reset` field replace on the same service id | N/A | N/A | collision warning; fragment without image is not an error | — |

**Current vs target:** F1–F3 and profile/`devices` rows match layer-4 code
(ADR-020, ADR-021). F5 round-trip coverage includes healthcheck CMD-SHELL
(`TestF5HealthcheckRoundTrip`) and SELinux bind / string-command fixtures.

## Interpolation: compose config vs import propose

| Mechanism | `docker compose config` | `iacpack import propose` |
| --- | --- | --- |
| Project `.env` | Yes | **No** (by design) |
| `--env-file` | Yes | **No** |
| Shell environment | Yes | **No** |
| `${VAR:-default}` in compose file | Yes | Defaults from compose text only → literal or parameter default |
| `${VAR:?err}` | Fails config if unset | Parameter gap; no secret values |

Operators who rely on `.env` for image tags or ports should either set parameter defaults in onboarding (ADR-018) or use compose-file defaults (`:-`).

## Profiles

Per [Using profiles](https://docs.docker.com/compose/profiles/):

- Services **without** `profiles` start on default `docker compose up`.
- Services **with** `profiles` start only when profile is active.

**Target (ADR-020 §6):** Propose accepts `activeProfiles` equivalent to `COMPOSE_PROFILES`. Default propose with empty `activeProfiles` imports only always-on services; profile-gated services are skipped with **info**, not **error**.

**Apply:** Rendered Compose may list `profiles` on services; operator must pass `--profile` to the provider when bringing up optional services (document in runbooks).

## GPU and devices

| Compose pattern | Documented for | iacpack v1 |
| --- | --- | --- |
| `deploy.resources.reservations.devices` + `capabilities: [gpu]` | Docker GPU guide | `resources.gpu` 1..8 or `all`, driver `nvidia` only (ADR-021) |
| `devices: ["nvidia.com/gpu=all"]` | CDI / Podman stacks | Policy deny; service excluded from draft |

`device_ids` and non-nvidia drivers are not imported. CDI `devices:` is not
translated into `resources.gpu`. On Podman apply, missing CDI
`nvidia.com/gpu=all` refuses apply with `APPLY.GPU_UNAVAILABLE` and does not
start the service on CPU ([runtime-apply-v1.md](runtime-apply-v1.md)).

## Paths and Prepare

Compose bind paths are relative to the compose file directory. iacpack stores workspace-relative `resources.*.source`.

Short mode `:z` and `:Z` are SELinux relabel flags and stay writable.
`readOnly: true` is recorded only when the mode contains `ro` (`:ro`, `:ro,z`).
A log directory without `:ro` stays writable. Render emits `read_only: false`
for writable binds, `read_only: true` only for RO, and `selinux: z` on payload
binds. Apply uses `--force-recreate` so an older read-only HostConfig is
replaced; a still-RO mount becomes `APPLY.ACTION` with service id and target.

- Missing directories at propose → `IMPORT.PATH_INVALID` **warning**; use import **Prepare** (stub directories) before validate/build.
- `../` paths with compose at workspace root → rebase warning; prefer `docker/stack/compose.yml` layout per Docker project directory rules.

## Partial import and Accept

Per ADR-020 §7:

- **Accept** is blocked only when `propose.ok` is false: parse failures, included-service errors, or empty draft.
- Policy-denied or profile-skipped services must **not** block Accept for an otherwise valid partial draft.
- Refusals (`policy`, `unsupported`, `secret`, `path`, `collision`) are review-only unless tied to **error** on an **included** service.

## Verification expectations (F5)

1. Golden propose fixtures from Docker doc snippets.
2. propose → render → YAML snapshot tests for emit subset.
3. Optional: `docker compose config` / `podman compose config` on rendered tree where supported.

## See also

- [QUICKSTART — Import](../release/QUICKSTART.md#import-existing-compose-draft)
- [KNOWN_LIMITATIONS — Import](../release/KNOWN_LIMITATIONS.md)
- [Manifest reference](manifest-v1alpha1.md)
