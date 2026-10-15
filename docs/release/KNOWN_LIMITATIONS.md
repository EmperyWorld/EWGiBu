# Known limitations (v1.1.0)

## Release readiness (2026-09-29)

Product version **`1.1.0`** delivers TZ §5.1–5.3 (v1.0 runtime + v1.1 authoring).
Feature phases P00–P19 are complete.

| Gate | Status on evidence host |
|------|-------------------------|
| `make check` / race / security | pass |
| Integration + Go E2E | pass |
| UI Playwright (incl. a11y) | pass (run `make test-ui`) |
| Reproduce double-build (Linux) | pass |
| Cross-OS reproduce CI matrix | workflow added — confirm green on GitHub |
| FULL=1 validate perf §19.2 | **pass** (p95 6 ms ≪ 2 s); see `PERFORMANCE.md` |
| Docker/Podman `compose config` / live apply | **pass** (2026-09-29, `engine=podman`; `IACPACK_DOCKER_APPLY=1`). Podman is E2E stand-in only — not a v1 provider |
| Threat model | reconfirmed 2026-09-29 — no blocker/critical |
| Signed public publish | human only — `PUBLISH.md` |

## Product limitations (by design)

- No CRDT / multi-user merge; disk conflicts require explicit recovery.
- Canonical Save may normalize YAML formatting and drop comments.
- File watcher is poll-based (~200–350 ms debounce).
- Core package coverage gate targets ≥90% statements for the declared core set;
  adapter/CLI surface uses scenario tests (see coverage report).
- React Flow vendor chrome may expose extra unlabeled controls; use dependency
  table + Connect/Disconnect for AT.
- Compose/Dockerfile import is **not** full Compose Spec fidelity. See
  [ADR-020](../adr/020-compose-import-fidelity-layers.md) and
  [compose-import-fidelity.md](../reference/compose-import-fidelity.md).
  Highlights: no ambient `.env` at propose (unlike `docker compose config`);
  profile-gated services require `activeProfiles` on propose (CLI `--profile`, API/UI);
  service-level `devices` / GPU CDI stay policy-denied. Deploy GPU count is
  `resources.gpu` (ADR-021); `device_ids` are not imported. `count: all` is
  `resources.gpu: all`. On Podman, apply refuses `gpu: all` when CDI
  `nvidia.com/gpu=all` is not declared (`APPLY.GPU_UNAVAILABLE`, mentions
  libcuda / CPU) and does not start containers. `gpu: devices-only` bind-mounts
  `/dev/nvidia*` (including `/dev/nvidia-caps`), sets `NVIDIA_VISIBLE_DEVICES`
  to the mounted card index, and on enforcing SELinux adds
  `security_opt: label=disable` for that service only. `void` hides the card
  from CUDA. A same-archive re-apply is not a noop when that host GPU stamp
  changes. Several local images in one tar need `podman save -m`; without it
  every name is a tag of the first image. Overrides merge is not Compose
  `include`/`extends`. Operator notes: [IMPORT_AUTHORING.md](IMPORT_AUTHORING.md).
- Apply (Podman/Docker CLI stand-in): each service is
  `compose up -d --no-deps --force-recreate <service>`. `service.ready` uses
  engine `ps`/`inspect` labels (not `compose ps --status/--services`, which
  podman-compose 1.6 rejects). A start or ready failure is per-service and does
  not abort the rest of the plan. Writable binds render as `read_only: false`;
  a still-RO mount is `APPLY.ACTION` with service id and path. YAML 1.1 bool
  argv (`yes`/`no`/`on`/`off`) is quoted on render. Details:
  [runtime-apply-v1.md](../reference/runtime-apply-v1.md).
- Compose import does not invent migration tasks. `IMPORT.MIGRATE_GAP` warns
  when a migrations tree is present; authors add `spec.tasks` and
  `service → migration completedSuccessfully` (ADR-009). Greenfield SQL uses
  `baseline.sql` + `pending/` only — not `applied/` replay.
- Empty deploy parameter strings of type integer/boolean/number fail render;
  empty strings omit Compose env keys.
- `spec.builds` runs `pnpm`/`npm`/`yarn`/`bun` during `iacpack build` only.
  `validate` checks `package.json`, the lockfile, and importers outside
  `context`. A path that leaves the workspace, or a missing importer, is
  `BUILD.FRONTEND` with that path. Directories that stay inside the workspace
  are copied with the relative path kept. The pnpm work directory is on the
  store filesystem, otherwise install uses `--package-import-method=copy`.
  Package-manager errors keep the tail of the log. `validate` does not install.
  Apply mounts `./payload/build-<id>`. An empty output fails the build.
- Compose `user:` imports as `service.user` (`name` or `uid:gid`). A
  `${VAR:-default}` imports the default when that default is a name or
  `uid:gid`. A missing Compose `user` is filled from local image `Config.User`
  or the last Dockerfile `USER` when that value is numeric. A name is reported
  (`IMPORT.USER_NAME`) and not written. An image that is present with an empty
  `USER` and no Dockerfile `USER` leaves `service.user` empty.
- `spec.databases` replays a SQL file when its SHA-256 changes. The file must
  tolerate a second execution. A new filename is the incremental path. The
  password never becomes a values entry.
- `iacpack bundle` exits 0 when the kit and `project.iacbundle` are written.
  Empty host paths and unembedded local-only images are warnings.
  `plan` / `apply` of the kit directory are the hard stop
  (`PORTABLE.HOST_PATH`, `PORTABLE.LOCAL_IMAGE`) until every value is an
  absolute path and every `load` / `local-only` image is present.
- Secret files are mode `0440`. Pass them with `--secrets-dir` (file name is the id) or `--secret-file`. Rootless chown runs for a numeric uid other than `0` / `0:0`. A name is not rewritten. On SELinux enforcing the secrets directory is labeled `container_file_t`. Task containers read the mounted file; they do not receive `IACPACK_SECRET_*` in the environment.
- Before state writes, apply reports `APPLY.IMAGE_PULL` for missing registry tags and stops with exit 6 on `APPLY.IMAGE_LOCAL`, `APPLY.HOST_PATH`, `SECRET.MISSING`, or `PARAM.MISSING`. `scripts/prepare-apply-host.sh` installs Podman and loads `images/*.tar`.
- An empty Compose argv element is `{literal: ""}` and is rendered as `- ""`.
  Secret interpolation in command or env is a secret reference or `parts`,
  not a dropped token.
- Lifecycle tasks use Compose profile `iacpack-task`. Apply and
  `iacpack task run` pass `--profile iacpack-task`. A task `entrypoint`
  replaces the image ENTRYPOINT (`[]` clears it); `command` is not an
  argument to that entrypoint unless `entrypoint` is omitted. `user` is a
  uid, `uid:gid`, or name. File secrets mount `../secrets/<id>` from the
  instance secrets directory.
- Loopback API/UI has no deploy/apply endpoints (CLI runtime remains separate).
- Windows installer / Authenticode not in scope.
- Public GitHub publish is human-gated only (no in-app push).

## Out of scope for v1.x

First-class Podman provider profile, Kubernetes/k3s, Windows/PowerShell
distribution, remote Docker contexts, host hooks/plugins, hosted multi-tenant UI,
automatic GitHub publish from the app (`TZ.md` §5.4, `SUPPORT_MATRIX.md`,
plan §6 / §8.3). Operator E2E may use Podman as a docker CLI/socket stand-in.
