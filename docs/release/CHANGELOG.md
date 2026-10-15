# Changelog

## Unreleased

### Added

- **Interactive launcher:** `scripts/launch.sh` shows a TTY menu for GUI
  (`serve`), `validate`, and `version`, with workspace and address prompts.
  Non-interactive callers keep using `scripts/run-product.sh`.
- **Secret file generator:** the Forms secrets section can classify an id as
  password, port, or name and write `secrets/<id>` in the workspace. The API
  response is the path only; the manifest stays a declaration.
- **Import image user / environmentFrom:** propose fills numeric `Config.User`
  into `service.user` (`IMPORT.USER_NAME` for names). A missing image falls
  back to the last Dockerfile `USER`, or warns `IMPORT.USER_ABSENT`. Re-import
  keeps an existing draft user. Same-image services are linked with
  `environmentFrom` plus override keys (`IMPORT.ENV_FROM`).
- **spec.databases:** extra DB on an existing server; compile emits `db-<id>`
  migration after `healthy`, applies SQL by name, rewrites `urlParameter` DSN
  with a password secret reference.
- **Portable kit apply:** `--embed-local-images` writes `images/*.tar` and
  `load` rows. `bundle` warns on an empty host-path template or a local-only
  image that was not embedded, and still writes `project.iacbundle`. `plan`
  and `apply` of the kit stop with `PORTABLE.HOST_PATH` /
  `PORTABLE.LOCAL_IMAGE` until paths are absolute and loads are present, then
  use the archive. A filled value replaces that host bind.
- **Secret mounts:** mode `0440` and chown to the service/task uid so non-root
  tasks can read secrets.
- **Stale lock:** `plan` and `apply` stop with `lock устарел` and
  `iacpack diff --from` when the workspace draft spec differs from the lock
  inside the bundle.
- **Volume ownership:** rootless Podman writable mounts of a numeric service
  user are `podman unshare chown` before start.
- **Task input identity:** every `oncePerDeployment` task reruns when command,
  entrypoint, user, or mount file bytes change. A `spec.databases` SQL file
  whose SHA-256 changed is applied again and must be safe to run twice.
- **environmentFrom:** a service copies another service's environment and
  overrides individual keys. `plan` lists missing keys (`PLAN.ENV_MISSING`).
- **`iacpack explain`:** image, gpu, `ldLibraryPath`, and devices for one
  service. An unhealthy service prints the last 80 log lines during apply.
- **Apply preflight:** before any state write, apply reports registry images it
  will pull (`APPLY.IMAGE_PULL`), local images pull cannot fetch
  (`APPLY.IMAGE_LOCAL`), missing host paths (`APPLY.HOST_PATH`), missing
  secret files, and deploy parameters with no value. Errors exit 6.
- **`--secrets-dir`:** `plan` and `apply` read one file per secret id. An
  explicit `--secret-file` overrides the same id. File bytes stay out of
  diagnostics and process arguments.
- **Operator guide:** [GUIDE.md](GUIDE.md) covers import `user` /
  `environmentFrom`, `spec.databases`, secrets, GPU, and the portable kit.

### Fixed

- **Apply:** an image `WorkingDir` is written as Compose `working_dir`. When the
  service also overrides `entrypoint`, the process runs `cd <workdir>` before
  that entrypoint, so Podman resetting the cwd to `/` does not hide
  `server.py` from `uvicorn`.
- **Apply:** `compose run` for a task uses `-T`. With a TTY, `psql` waits for
  a password and apply does not continue. The task `timeout` from the manifest
  is kept (`30m` for a large SQL baseline is not cut to five minutes), and
  task output is streamed. Creating a missing `POSTGRES_DB` runs only in the
  Postgres server image. API and worker containers that merely point at that
  database are left alone.
- **Apply:** an image `USER` of `0` or `0:0` is container root and does not fail
  ownership. Host binds, including a read-only models directory, are labeled
  `container_file_t`. `devices-only` also mounts `/dev/nvidia-caps` device
  nodes. After Postgres is healthy, a missing `POSTGRES_DB` is created when
  the data directory already existed and init was skipped.
- **GPU devices-only:** bind-mounts `/dev/nvidia*` and sets `LD_LIBRARY_PATH`.
  `NVIDIA_VISIBLE_DEVICES` is the mounted card index (`0` for `/dev/nvidia0`),
  not `all` (that requests CDI) and not `void` (that hides the card from CUDA).
  On enforcing SELinux the service also sets `security_opt: label=disable`,
  because `container_t` cannot open `/dev/nvidia*`.
  `gpu: all` still refuses apply when that CDI device is missing.
- **Lock:** `PARAM.SECRET_LIKE` matches `token` as its own name segment.
  `llama-chat-image-max-tokens` is not a secret (`tokens` is a different word).
  `jitsi-token-ttl-seconds` is a duration (`token-ttl`), not a credential.
  `api-token` in a values file is still rejected.
- **Doctor:** rootless Podman accepts podman-compose 1.x (the 2.24 minimum is
  the Docker Compose plugin only) and probes the engine with
  `{{.Version.Version}}` instead of Docker's `ServerVersion`.
- **Import:** environment hosts that match a Compose `container_name` are
  rewritten to the service id (`stack1-seaweedfs` → `seaweedfs`). `plan`
  rejects a remaining single-label host that is not a service
  (`PLAN.UNKNOWN_HOST`). Env keys are not renamed (`CORS_ORIGINS` stays
  `CORS_ORIGINS`). A healthcheck element that is an unresolved `${VAR}` or an
  empty string is an import error; the probe is not rendered as `curl -f`
  without a URL.
- **Tasks:** `entrypoint` replaces the image ENTRYPOINT (`[]` clears it) and
  `user` (`uid`, `uid:gid`, or name) is rendered on the task service. `command`
  stays the argument list, so a Keycloak image does not receive the task
  script as a `kc.sh` option.
- **Build:** `spec.builds` runs `pnpm`/`npm`/`yarn`/`bun` during `iacpack build`
  (frozen lockfile, `package.json` script). Workspace packages outside
  `context` that the lockfile, `pnpm-workspace.yaml`, or a relative source
  import names are copied with their relative path. A missing or escaping
  importer is `BUILD.FRONTEND` before install. The pnpm work directory is on the store filesystem, or
  install uses `--package-import-method=copy`. Package-manager errors keep
  the tail of the log. The output, including `index.html`, is copied into
  the bundle payload. Apply mounts `build: <id>` and does not install
  packages. An empty output fails the build.
- **Import:** an empty Compose command element (`""`) is kept as `{literal: ""}`
  and rendered as an empty argv element (`--logfile ""`).
- **Apply:** Compose file secrets mount `../secrets/<id>` (mode 0440 under the
  instance secrets directory). A missing `--secret-file` fails before
  `compose run` or `compose up`, instead of a podman `statfs` on
  `runtime-secrets/`.
- **Import:** Compose `${SECRET}` in command argv and environment is a secret
  reference (`{secret: id}` or a `parts` element), not a dropped token. An env
  value may concatenate literals and a secret (for example a Redis URL).
  `service.user` (`name` or `uid:gid`) is imported and rendered as Compose `user:`.
- **Apply:** `compose run` for lifecycle tasks passes `--profile iacpack-task`
  so podman-compose and Docker Compose execute profiled task services.
- **Import:** a Compose bind with `:z` or `:Z` stays writable. `readOnly: true` is
  set only when the source mode contains `:ro`. A string `command` or
  `entrypoint` such as `command: start` is kept as argv and rendered into the
  container.
- **Render:** empty string deploy parameters of type `integer` / `number` /
  `boolean` fail at render; empty `string`/`enum` values are **omitted** from
  Compose `environment` (no `KEY: ""`).
- **Render:** argv values that are YAML 1.1 booleans (`yes`/`no`/`on`/`off`/
  `true`/`false` and case variants) are quoted in Compose YAML so podman-compose
  passes the original strings into the container. Writable binds emit
  `read_only: false` explicitly.
- **Apply:** `service.ready` polls the engine with `ps`/`inspect` labels instead of
  `compose ps --status running --services` (rejected by podman-compose 1.6). A
  readiness timeout no longer aborts the rest of the plan. `compose up` always
  uses `--force-recreate` so prior read-only HostConfig is replaced. After start,
  a mount that should be writable but is RO becomes `APPLY.ACTION` with the
  service id and target path.
- **Apply:** Podman `driver: nvidia` / `count: all` is not passed as CDI
  `nvidia.com/gpu=all` when that device is not declared on the host. Apply
  records `APPLY.GPU_UNAVAILABLE` with the service id and that device name, and
  starts the container without the reservation. Each service is
  `compose up -d --no-deps --force-recreate <service>`, so one failure does not
  replace the other services with a single compose-up banner.

### Changed

- **Lifecycle (ADR-009 / TZ §9.11):** a service may `dependsOn` a `migration`
  task with `completedSuccessfully` so apply starts DB-bound apps only after
  migrate succeeds. `service → postDeploy` remains forbidden.

### Added

- **Import propose:** workspace trees matching `**/migrations` with
  `baseline.sql` and/or `pending/` report `IMPORT.MIGRATE_GAP` (warning) with a
  task + API `dependsOn` YAML template. Compose import still does not invent
  `spec.tasks`. Greenfield contract: one `baseline.sql` + `pending/`; `applied/`
  is history and is not replayed. Example runner:
  `docs/release/examples/pack-migrate`.
- **Lock:** the same workspace file mounted by several resources is packed into each
  resource payload. A path that is a directory on disk and a file in the bundle is
  replaced by that file on apply.
- **Lock and apply:** a workspace file with an execute bit is stored and extracted
  as mode `0755`. Other files stay `0644`. Payload directories stay `container_file_t`.
- **Apply:** a 0-byte workspace file is stored and extracted as an empty file.
  Missing content is `INPUT.IO_ERROR` only when the entry could not be read or written.
- **Apply:** file binds are mounted as files (`payload/<id>/<file>`), not as the
  resource directory. Before `compose up`, rendered payload is labeled
  `container_file_t` and payload binds use Compose `selinux: z`.
- **Import propose:** a required host bind `${VAR:?…}` is an external mount
  (`host: {parameter}`) and is not bundled. A command argument
  `/models/${FILE:?…}` stays as `{parts: ["/models/", {parameter}]}` and becomes
  `/models/<file>` when the parameter is set at apply. `${IMAGE_MAX_TOKENS:-512}`
  is that count parameter (default 512), not a secret named token.
- **Import propose:** secret-like environment keys stay in the draft as
  `{secret: id}` and `spec.secrets`. The id comes from `${VAR}`, not the key
  name, including when they differ (`RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD}`).
  Values are not read. `env_file` ids follow the file path. Numeric
  settings such as `JITSI_TOKEN_TTL_SECONDS` stay parameters. Apply passes the
  file with `--secret-file id=/path` and does not put the value on the command line.
- **validate/lock/build:** a manifest in `imports/` uses the parent directory as the
  workspace root, so `imports/import-draft.json` resolves `shared/api` at the
  project root. Include patterns limit the input walk. A file source stays a
  file. Hardlinks and in-workspace symlinks are accepted; a symlink that leaves
  the workspace is refused. The same workspace path in two resources is stored
  once and is not `INPUT.PATH_COLLISION`.
- **Import propose:** Compose `$$` in a command unfolds to a literal `$` before
  `${…}` is rejected. A `CMD-SHELL` healthcheck such as `$${POSTGRES_USER}` stays
  in the draft. `depends_on` `service_healthy` is kept only when that target
  still has a healthcheck; otherwise the condition becomes `started` (warning).
- **Import propose:** `spec.images.*.ref` may be `{parameter: id}` when the image
  has no literal default. That draft validates. Nested `${A:-${B:-…}}` peels to
  a literal ref, or to a parameter with no default when no literal remains.
- **Import propose:** a Compose `!override` / `!reset` fragment merges into the
  same service id whichever file is first. Missing `image` / `build` on that
  fragment is not `IMPORT.EMPTY`. An absolute host bind is a policy warning
  (error only with `--import-strict`); the mount is omitted and the service stays.
- **Import propose:** whole-value `image: ${VAR}` and `image: ${VAR:?message}` become a
  parameter gap and keep the service (`propose.ok` stays true). Compose
  `!override` / `!reset` parse on import inputs only; iacpack manifests still
  forbid custom tags.
- **Import follow-ups:** service `profiles` are stored and rendered; GPU `count: all`
  maps to `resources.gpu: all`; workspace directory listing
  `GET /api/v1/workspace/files`; `.tar.gz` upload; larger upload body on that
  route. `../` binds stay inside the workspace (hint only; no host escape).
- **ADR-021 Deploy GPU:** manifest `resources.gpu` (1..8) imports and renders
  Compose `deploy.resources.reservations.devices` (`driver: nvidia`,
  `capabilities: [gpu]`). Service `devices:` / CDI stays excluded. Fixture
  `testdata/import/f3/compose.yaml`.
- **Import fixture `testdata/ui/two-service/stack4.yml`:** flow-style service lists and
  bind paths so stub preview/prepare tests run without an external stack.
- **ADR-020 F5 slice:** propose healthcheck argv round-trips to Compose `CMD-SHELL`.
- **Import workspace upload:** `POST /api/v1/workspace/upload` (`file`, `tree`, `zip`)
  for explicit copies into the workspace. Import sources and bind rows offer
  upload file, folder, or `.zip` according to the expected path kind. Folder
  picks land inside the target directory (selected folder name is not nested).
  Import draft dialog uses the full viewport width.
- **ADR-020 F2 propose semantics:** partial `propose.ok` when draft has content
  and only warnings/info (policy-excluded services, profile skips); optional
  `activeProfiles` / `--profile` and `strict` / `--import-strict`; diagnostic
  `IMPORT.PROFILE_SKIPPED`; HTTP `POST /api/v1/import/propose` fields
  `activeProfiles`, `strict`; import wizard Compose profiles field; fixture
  `testdata/import/f2/compose.yaml`.
- **ADR-020 F1 import parity:** image `repo:${TAG:-default}` resolution; service
  `mem_limit` → manifest `resources.memory` (with deploy conflict detection);
  `deploy.resources.limits.memory` via shared parser; healthcheck `CMD-SHELL` →
  manifest `["/bin/sh","-c",script]` and render back to `CMD-SHELL`; bind sources
  with `${VAR:-path}`; fixture `testdata/import/f1/compose.yaml`.

### Documentation

- ADR-020: Compose import fidelity, four reference layers, partial propose/Accept
  severities, profiles/GPU/interpolation rules, implementation phases F0–F5.
- ADR-021: Deploy GPU reservations (`resources.gpu`).
- Operator note: `docs/release/IMPORT_AUTHORING.md` (wizard, uploads, digests,
  lock drift, `scripts/run-product.sh`) and `docs/release/CONSOLE.md` (terminal workflow).
- Reference: `docs/reference/compose-import-fidelity.md` (capability matrix).
- QUICKSTART, KNOWN_LIMITATIONS, TZ §9.8 / §9.15 / §10, ADR-016/017 amendments.

## 1.1.0 (v1.0 + v1.1 stable)

First public stable line covering TZ §5.1–5.3 (MVP → v1.0 runtime + v1.1
authoring). Feature phases P00–P19 are included. Remaining human/CI gates
(cross-OS builders, signed publish) are in `KNOWN_LIMITATIONS.md` /
`TRACEABILITY.md` / `PUBLISH.md`.

### Added

- End-to-end packaging CLI: init, validate, lock, build, verify, render, plan,
  apply, resume, status, destroy, doctor, images, migrate, serve.
- Loopback authoring API + embedded Web UI (graph designer, forms, preview,
  CSRF/ETag saves, disk sync SSE).
- Deterministic directory/bundle artifacts, SBOM/provenance rehearsal, offline
  OCI layout support.
- Normative schemas, ADRs 001–020, reference docs, release evidence pack.
- Compose/Dockerfile **import propose** drafts (P16): `import propose` CLI,
  `POST /api/v1/import/propose`, UI Propose → Accept (memory) → explicit Save;
  subset inverse of compose-docker/v1 plus external nets/vols (ADR-016).
- Deeper import onboarding (P17): widened discover (`compose.*`,
  `docker-compose.*`, `stack*`); multi-Compose first-wins merge; structured
  `refusals[]`; UI multi-select + refusal groups (ADR-017).
- Secrets/parameters onboarding (P18): `parameterGaps[]`, `secretSuggestions[]`,
  `POST /api/v1/import/onboard`, placeholder-only `.env.example` (ADR-018).
- GitHub release templates (P19): `iacpack release init-templates`,
  `POST /api/v1/release/templates/{propose,write}`, pinned CI workflow helpers,
  no auto-publish (ADR-019).
- Operator container-engine E2E helper: `scripts/p15/docker-e2e.sh` (Docker or
  Podman docker-CLI stand-in; optional `IACPACK_DOCKER_APPLY=1`).
- Real `destroy` host path: `docker compose … down` via `HostDestroy` (no longer
  a recording stub); optional `--docker-socket`.
- Real `task run` host path via `HostApply` + Compose profile `iacpack-task` for
  one-shot lifecycle tasks; `rerun: manual` skips apply auto-execution.
- Authoring onboarding: `GET /api/v1/workspace`, welcome empty state, Save As
  dialog (no `window.prompt`), **Deploy in terminal** CLI script panel (opens after Save).
- Compose import: flow-style YAML (`networks: [a]`, inline `command`/`healthcheck`),
  `${VAR:?…}` env lines, list-form `environment`, compose-file-relative `build.context`,
  and omit (not deny) `security_opt`.
- Compose import short syntax: string `ports` (`127.0.0.1:host:container`, `${…}` host)
  and string `volumes` (`../path:/container:z`) with compose-relative bind resolution.
- Compose flow collections accept a **trailing comma** before `]`/`}` (multiline `command:` lists).
- Compose import resolves `image: ${VAR:-default}` to the default ref (fixed pull); bare `${VAR}` still refused.
- Runtime apply/destroy/task/doctor: **Podman first** auto engine selection; `--container-engine podman|docker` and `IACPACK_CONTAINER_ENGINE`.
- Compose import: string ports with `${VAR:-bind}:host:container` no longer truncate bind at `:-`; defaults become literal bind addresses.
- Import UX polish: deduped propose diagnostics, missing bind-source warnings, memory
  limits as bytes, softer “review refusals” notice vs blocked errors, collapsed unsupported refusals.
- `POST /api/v1/import/resource-stubs` and `iacpack import resource-stubs`: create empty
  bind directories, image `build.context` directories, and zero-byte file stubs from
  import drafts (UI auto re-proposes after success); file binds use parent `source` + `include`.
- `POST /api/v1/import/stub-preview` and `iacpack import stub-preview`: dry-run stub plan;
  import panel shows preview + CLI recipe; compose `../` paths at workspace root rebase to
  workspace-relative bind/build paths (warning diagnostic).
- `POST /api/v1/import/write-draft` and `iacpack import write-draft`: write canonical draft
  Project JSON (default `imports/import-draft.json`) for CLI stub commands; UI **Save draft JSON**,
  **Download draft JSON**, **Save JSON + create stubs**, and copy CLI recipe.
- `POST /api/v1/import/prepare` and `iacpack import prepare`: write draft JSON and create stubs
  in one step; import panel uses it for Prepare, shows path counts, collapsible diagnostics, and
  editable draft path + stub preview refresh.
- Import **beginner wizard** (sources → review → folders → secrets → finish), draft status
  chips, diagnostic actions, Accept gate explanations, and post-Accept next-steps banner (Save/Validate).
- Import wizard hides the **Folders** step when the draft has no bind/build work; welcome copy matches
  the wizard; focus moves to **Next** after Propose.

### Security

- Default-deny privileged containers, host namespaces, Docker socket mounts,
  host hooks; secret canaries; path/archive confinement; loopback-only UI.

### Known issues

See `docs/release/KNOWN_LIMITATIONS.md` (remaining **release gates**: cross-OS
CI `reproduce-multi-os` green confirmation, human signed publish). Container
E2E §23 #10/#12 closed on evidence host with Podman stand-in (2026-09-29).

## 0.1.0-rc.1 (release candidate)

Superseded by `1.1.0`. Kept for history.
## Upgrade notes

From development snapshots: regenerate lockfiles (`iacpack lock`), rebuild
artifacts, review operator policy before apply. No automated in-place upgrade of
running Compose projects beyond documented apply/resume.
