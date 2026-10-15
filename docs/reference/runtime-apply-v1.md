# Runtime apply (v1)

`iacpack apply` verifies a bundle, plans, acquires the instance lock, journals
each action, materializes Compose/secrets under the XDG state root, and invokes
the local Compose-compatible engine (Podman preferred when present, else Docker)
with a sanitized environment.

## Partial failure

Apply is **resumable with compensating cleanup**, not atomic rollback. Exit `6`
means failure before mutation; exit `7` means mutation occurred. Volumes and
pre-existing resources are never auto-deleted. See ADR-008.

`service.start` and `service.ready` failures are **per-service**. A failed start
or readiness wait records `APPLY.ACTION` (or `APPLY.GPU_UNAVAILABLE` for CDI)
with the service id, marks that service failed, and **continues** later
independent actions. A readiness timeout on one service must not look like a
GPU/start failure on another. See [graph-v1.md](graph-v1.md).

## Service start

Each `service.start` is:

```text
compose -p <slug> -f <compose> up -d --no-deps --force-recreate <service>
```

Lifecycle tasks are profiled `iacpack-task` and run as:

```text
compose --profile iacpack-task -p <slug> -f <compose> run --rm --no-deps <task>
```

`--profile` is required: without it podman-compose reports the task as a missing service.

`command` is the Compose command, not a replacement for the image `ENTRYPOINT`.
A task may set `entrypoint` (same argv as a service). That value is rendered as
Compose `entrypoint`. `entrypoint: []` clears the image entrypoint so `command`
is the executable. Without the field, the image entrypoint stays (for example
Keycloak `kc.sh` treats the script path as an unknown option). `user` is a
uid (`0`), `uid:gid`, or name and is rendered as Compose `user`.

`--force-recreate` replaces containers whose HostConfig still has read-only
binds or stale GPU devices from a previous apply. One create failure stays on
that service.

After a successful up, apply inspects mounts. If a path that must be writable
(`readOnly: false` in IR) is mounted read-only, apply records
`APPLY.ACTION` as `service "<id>": mount "<target>" is read-only` and treats
the start as failed for that service.

### GPU `count: all` (Podman)

See [ADR-021](../adr/021-deploy-gpu-reservations.md). `gpu: all` requires CDI
`nvidia.com/gpu=all`. When that device is missing, apply stops before any
container starts and records `APPLY.GPU_UNAVAILABLE`: devices without
`libcuda` would run on CPU.

`gpu: devices-only` with `ldLibraryPath` does not use CDI. Apply bind-mounts
host `/dev/nvidia*` nodes, including character devices under `/dev/nvidia-caps`,
and sets `LD_LIBRARY_PATH` to that directory (for
example `/usr/local/cuda/compat` inside the image). `NVIDIA_VISIBLE_DEVICES`
is the index list of the mounted cards (`nvidia0` becomes `0`), not `all`
and not `void`. `void` hides the mounted cards from CUDA. On an enforcing
SELinux host that service also gets `security_opt: [label=disable]`, because
`container_t` cannot open `/dev/nvidia*` (`xserver_misc_device_t`) and CUDA
then reports that no device exists. That one container is not confined.
`NVIDIA_DRIVER_CAPABILITIES` is not overridden, so the image default remains.

## Stale lock

`plan` and `apply` compare the workspace draft with the lock stored in the
bundle when `--workspace` is set, or when `./iacpack.yaml` names the same
project. A different semantic hash stops before mutation with `LOCK.DRIFT`,
message `lock устарел`, and hint `iacpack diff --from <manifest> --to <lock or bundle>`.

## Writable mount ownership

On rootless Podman, files owned by the host user appear as root inside the
container. Before `compose up`, apply chowns each writable bind to the
service uid. If `service.user` is empty and the image `Config.User` is empty,
as with library Postgres, the entrypoint drops to uid 999 before opening
`/logs`. Apply then sets mode `0777` on that payload directory so the
process can create the log file. A host path such as the model directory is
left unchanged. Docker is left unchanged. A non-numeric user name that cannot
be resolved is not rewritten.

## Task input identity

Every `rerun: oncePerDeployment` task reruns when the hash of `command`,
`entrypoint`, `user`, or mount file bytes changes, including a host script
whose path string did not change. The last successful hash is
`task-inputs.json` under the instance state directory. It stores no secret
values. A new bundle still skips a task whose own hash matches. `always` runs
every apply. `manual` runs only from `iacpack task run`. `compose run` uses
`-T`, so `psql` does not wait on a TTY for a password. The task's own
`timeout` is the limit, and its output is streamed while it runs.

After a Postgres server image is healthy, apply creates `POSTGRES_DB` when
that database is missing (init was skipped because the data directory already
existed). A service that only has `POSTGRES_DB` in its environment is not
used for this: the client image has no `psql`. A second database name is
created only when the bundle contains `spec.databases`.

## environmentFrom

A service may set `environmentFrom: <service>`. Render copies that service's
environment, then applies the child's `environment` keys. `plan` lists keys
present on another service with the same image and absent here
(`PLAN.ENV_MISSING`), including `LLAMA_MODEL` when a worker does not inherit
the API environment.

## Unhealthy log tail

When `service.ready` fails, apply appends the last 80 lines of that service's
container log to the `APPLY.ACTION` diagnostic.

## Networks

A network marked `external` is not created by Compose. Apply runs `network create` for that engine name when it is missing, before `compose up`. A managed network is still created by Compose.

## Portable kit

`iacpack bundle --workspace <dir> --output-dir <dir> [--policy <path>] [--embed-local-images]`
writes `iacpack.lock.json`, `values.json` (deploy defaults, secret-like names
omitted), `secrets.txt` (required secret ids, no values), `images.txt`
(`pull`, `local-only`, or `load`), `host-paths.json`, and `project.iacbundle`.
`--embed-local-images` saves each local-only image as `images/*.tar` and marks
the row `load`. `host-paths.json` is a template: every `value` is empty, and a
literal mount gets its own parameter id (`host-<service>-<target>`). Empty
values and images that stayed `local-only` are warnings on `bundle`. A failed
`save` stays an error.

`plan <kit-dir>` and `apply <kit-dir>` are the gate. One `PORTABLE.HOST_PATH`
lists every empty or non-absolute value. One `PORTABLE.LOCAL_IMAGE` lists
every `local-only` or `load` ref the engine does not have. After those checks,
both commands use `project.iacbundle`. A filled `value` replaces that
service or task host bind, including a path that was a literal in the
manifest. An operator `--set` for the same parameter is left in place.
A later apply of the same archive noops when those filled paths are
unchanged, and applies again when a value changes. It also applies again
when the host GPU device list changes, or when devices-only stops hiding
those devices (`NVIDIA_VISIBLE_DEVICES=void`). The bundle digest does not
include that host fact. Secrets stay names only.

`iacpack explain <bundle> <service>` prints image, gpu, `ldLibraryPath`, and
devices.

## Service ready

`service.ready` exists only when the service declares a healthcheck. Apply does
**not** call `compose ps --status running --services` (podman-compose 1.6
rejects those flags). It polls the engine with:

```text
ps -aq --filter label=com.docker.compose.project=<slug>
       --filter label=com.docker.compose.service=<id>
       [--filter status=running]
inspect --format '{{.State.Status}} {{.State.Health.Status}}'
```

(and the `io.podman.compose.*` label pair as a fallback). Ready means
`running` and, when a healthcheck is configured on the container, `healthy`.
Default wait budget: **2 minutes**. Timeout yields
`APPLY.ACTION` / `readiness timeout for <service>` and does not abort the rest
of the plan.

## Rendered Compose (apply materialization)

Apply re-renders Compose from the verified deployment IR before mutating.

- Writable binds emit `read_only: false`. Payload binds and host binds
  (including a read-only models directory) emit `bind.selinux: z`.
  Apply also labels each host bind path `container_file_t` before start.
  A read-only models directory is included. Without that label, SELinux
  reports `Permission denied` on a file the container user can read.
  Read-only binds emit `read_only: true` only when IR `readOnly` is true
  (Compose source `:ro`).
- Argv / scalar values that are YAML 1.1 booleans (`yes`/`no`/`on`/`off`/
  `true`/`false` and case variants) are **quoted** so podman-compose / PyYAML
  pass the original strings into the container (for example Redis
  `--appendonly "yes"`). An explicit empty argv element `{literal: ""}` is
  rendered as `- ""` so a flag such as `--logfile` keeps its empty value.
- `service.user` and `task.user` are emitted as Compose `user:` (`uid`,
  `uid:gid`, or name).
- A task `entrypoint` is emitted as Compose `entrypoint`. An empty array is
  `entrypoint: []` and clears the image ENTRYPOINT. `command` stays the
  argument list. Apply also sets `working_dir` from the image
  `Config.WorkingDir` when the manifest does not name one. If the service or
  task sets `entrypoint`, render runs `cd <workdir> && exec "$@"` before that
  entrypoint. A relative program such as `uvicorn server:app` then imports
  from the image directory even when the engine starts the process in `/`.
- A mount `build: <id>` is `./payload/build-<id>` (files produced by
  `iacpack build`, not by apply).
- Secret argv and concatenated env parts are Compose placeholders
  `${IACPACK_SECRET_<ID>}`. Apply sets that variable from the secret file.
  The secret bytes are not written into the compose file.

Import of `:z` / string `command` and the fidelity matrix:
[compose-import-fidelity.md](compose-import-fidelity.md).

## Secrets

`--secrets-dir <dir>` binds each regular file name to a secret id. `--secret-file id=path` overrides that id. Dotfiles and subdirectories are skipped. Values are copied to the instance `secrets/` directory as mode `0440` under `0700`. On SELinux enforcing, apply labels that directory `container_file_t` so the container can read the files. The relabel runs
as `podman unshare chcon`, because a data directory chowned to the image uid
(Postgres `pgdata` is mode `0700`) is not readable by the host user. Before `compose up` / `compose run`, apply chowns those files to the service or task numeric `user` (`podman unshare` on rootless Podman) so a non-root uid can read them. Mode is never wider than `0440`. A `spec.databases` migration task runs as that numeric user, or as `0` when the server service has no numeric user, so it can read the password file. Rendered Compose file mounts use `../secrets/<id>` relative to `rendered/compose.yaml` (the same directory services and tasks share). Before `compose up` or `compose run`, apply checks that every mounted secret file is readable and fails with `SECRET.INVALID_FILE` if it is not. Structured state, journal, logs, plan, and process arguments must not contain secret bytes. `destroy` removes the secrets directory.

## Apply preflight

After the bundle is verified and before the instance state directory is created, apply reports:

| Code | Severity | Meaning |
| --- | --- | --- |
| `APPLY.IMAGE_PULL` | info | Registry tag is missing locally. The hint is `podman pull` with the names separated by spaces, and apply runs that pull before writing state |
| `APPLY.IMAGE_LOCAL` | error | Local tag (`localhost/…` or a name with no registry) is missing. `pull` will not fetch it. Load a tar or build the tag |
| `APPLY.HOST_PATH` | error | A host bind value is not an existing absolute path |
| `SECRET.MISSING` | error | A required secret has no file |
| `PARAM.MISSING` | error | A deploy parameter still referenced by the deployment has no default, `--values`, or `--set` |

Any error exits `6` with no state write. `scripts/prepare-apply-host.sh` installs Podman and `podman-compose`, loads `images/*.tar` when present, and checks the binary, secret files, model path, and `/dev/nvidia0`. It does not start containers.

## Migration tasks (SQL greenfield)

Compose import does **not** create `spec.tasks`. Propose emits
`IMPORT.MIGRATE_GAP` when it finds `**/migrations` with `baseline.sql` and/or
`pending/`. Authors add a `phase: migration` task (`command` argv; optional
`entrypoint` and `user`) and wire
DB-bound services with `dependsOn: { target: <task>, condition: completedSuccessfully }`
(ADR-009).

| Path | Role |
| --- | --- |
| `baseline.sql` | Full schema dump for an empty database |
| `pending/*.sql` | Incremental migrations |
| `applied/` | Product history — **ignored** by pack migrate |
| tracking table | `iacpack_schema_migrations` (see `docs/release/examples/pack-migrate`) |

`spec.databases` compiles a separate `db-<id>` task. Its script records
SHA-256 of each SQL file in `iacpack_schema_migrations`. A changed file runs
again and must tolerate a second execution; an unchanged file is skipped.
`oncePerDeployment` also skips the task container when that task's input hash
matches. Partial failure: fix the SQL, then `iacpack task run <project> <task>`
or re-apply. Do not use host `podman exec` / `USE_DOCKER=1` scripts inside tasks.
