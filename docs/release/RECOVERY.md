# Deployment recovery

## Journal and resume

Apply records transitions in instance journal state (`docs/adr/008`,
`docs/reference/runtime-apply-v1.md`). After interruption:

```bash
iacpack status --instance <id>
iacpack resume --instance <id>
```

Resume classifies the last action as completed / pending / unknown and refuses
silent skip of unknown steps.

## Failed migrate task

- Migration tasks are one-shot containers (`iacpack task run` / apply graph).
- Greenfield uses `baseline.sql` once, then `pending/*.sql`, with rows in
  `iacpack_schema_migrations`. Do **not** replay `applied/`.
- After a failed migrate: fix the SQL, keep tracking consistent (failed file
  must not be recorded), then `iacpack task run <project> <task>` or re-apply.
- Services that `dependsOn` the migrate task with `completedSuccessfully` will
  not start until migrate succeeds.
- The task container is profile `iacpack-task`. Apply and `task run` pass
  `--profile iacpack-task`. Set `entrypoint` when the image ENTRYPOINT would
  swallow `command` (Keycloak `kc.sh`). Set `user` when the task must read a
  root-owned secret file. A file secret must already be supplied with
  `--secrets-dir` or `--secret-file`; Compose mounts `../secrets/<id>`.

## Failed apply

- Exit codes and diagnostics name the failed stage and, for per-service
  start/ready/mount/GPU failures, the **service id** (and CDI device when
  relevant). See [runtime-apply-v1.md](../reference/runtime-apply-v1.md).
- A `service.ready` timeout or `service.start` failure does **not** stop
  unrelated later services; exit `7` still means mutation occurred.
- Compensation does **not** delete pre-existing resources or unmanaged volumes.
- Fix the cause, then `plan` → `apply` again (`--force-recreate` on each
  service start replaces stale HostConfig).

## Destroy

```bash
iacpack destroy <project> [--instance <id>]                 # retain volumes
iacpack destroy <project> [--instance <id>] --delete-volumes --yes  # explicit data loss
```

## Manifest migration

```bash
iacpack manifest migrate --file iacpack.yaml
```

Migrator backs up metadata only; review diff before commit. Rollback is undo of
the migration file write, not infrastructure rewind.

## Bundle verify

```bash
iacpack verify path/to/artifact.iacbundle
```

Rejects forged hashes, duplicates, traversal, quota violations.
