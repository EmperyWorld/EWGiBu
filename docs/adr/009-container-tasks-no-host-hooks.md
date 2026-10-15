# ADR-009: Container lifecycle tasks and no host hooks

## Status

Accepted

## Context

Lifecycle work (warmup, migrations, post-deploy hooks) must run under the same
isolation and policy model as services. Host shell hooks would break the pure
compiler boundary, defeat secret redaction, and make packages non-portable.

Dependencies between services and tasks need a frozen phase partial order so
graph compilation and runtime planning stay deterministic and reviewable.

## Decision

1. **Tasks are one-shot containers**, never host processes. They use the same
   image/mount/network/secret rules as services, with argv `command` only
   (no shell strings).
2. **No host hooks** in v1 authoring or runtime. Compiler and planner never
   invoke `sh -c`, host scripts, or workspace executables.
3. **Frozen phase partial order** (TZ.md §9.11):

| Dependent | Allowed prerequisite |
|---|---|
| `preStart` task | other `preStart` task |
| service | `preStart` task, service (`started`/`healthy`), or `migration` task (`completedSuccessfully` only) |
| `migration` task | service or `migration` task |
| `postDeploy` task | service, `migration` task, or `postDeploy` task |

   `completedSuccessfully` targets a task of the same or earlier phase only.
   A service may wait on a migration barrier so DB-bound apps start only after
   schema apply succeeds. Ordinary network membership is not an execution
   dependency.
4. **Automatic prerequisites** for container actions: verify/preflight, image
   availability, managed networks/volumes, and declared resources/payload.
5. **Rerun policies**: `always`, `oncePerDeployment`, `manual` (`iacpack task run`
   only). Every `oncePerDeployment` task is keyed by the hash of `command`,
   `entrypoint`, `user`, and mount file bytes (not secret bytes). A changed
   script reruns; an unchanged script does not, even when the bundle digest
   changed. Default failure policy is `stop`.
6. **Action graph** is a DAG with edges prerequisite → dependent, stable action
   IDs, and lexicographic tie-breaking for ready nodes.
7. **Rendered tasks** are Compose services with `profiles: [iacpack-task]` and
   `restart: no`. `apply` and `iacpack task run` pass
   `--profile iacpack-task` to `compose run --rm --no-deps`. Default
   `compose up` does not start them.
8. **`entrypoint` and `user`** are optional task fields with the same meaning as
   on a service. `entrypoint` replaces the image ENTRYPOINT; `[]` clears it.
   `command` is not passed to the image entrypoint unless `entrypoint` is
   omitted. `user` is a uid, `uid:gid`, or name.

## Consequences

- Graph inspection (`inspect graph`) can be pure and offline.
- Idempotency of application migrations remains the author's responsibility;
  the tool only guarantees scheduling semantics and hash-keyed `oncePerDeployment`.
  Greenfield SQL convention: product-owned `baseline.sql` + `pending/`; do not
  replay historical `applied/` trees (see `docs/release/examples/pack-migrate`).
- A service may wait on `migration` via `completedSuccessfully` so schema
  apply is an apply-graph barrier for DB-bound apps.
- Host-side automation is out of scope until a future ADR with a threat model.

## Rejected alternatives

- Host `preStart`/`postDeploy` scripts — arbitrary code execution.
- Inferring dependencies from shared networks — hidden, non-reviewable edges.
- `continue-on-error` / automatic irreversible rollback — deferred beyond v1.
