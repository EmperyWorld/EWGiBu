# Action graph and deployment IR — v1

Normative deployment document: [`schemas/deployment/v1/schema.json`](../../schemas/deployment/v1/schema.json)  
(`$id`: `iacpack.schemas.deployment.v1`). Decisions: [ADR-009](../adr/009-container-tasks-no-host-hooks.md), TZ.md §§9.11–9.12.

## Compilation

After full validation (schema, semantics, parameters, inputs, policy), the
compiler builds:

1. **Immutable IR** — redacted deployment model (no `x-ui`, no absolute host
   paths, no secret values, no authoring source locations).
2. **Action DAG** — deterministic prerequisites for verify, provision, images,
   tasks, service start/readiness, and finalize.

`iacpack inspect graph` renders the same DTO as text, JSON, and Mermaid. It
executes no tasks, Docker, or network I/O.

## Action kinds

| Kind | Stable ID pattern |
|---|---|
| `verify` | `action:verify` |
| `provision.network` | `action:network/<id>` |
| `provision.volume` | `action:volume/<id>` |
| `provision.resource` | `action:resource/<id>` |
| `image.available` | `action:image/<id>` |
| `task.run` | `action:task/<id>` |
| `service.start` | `action:service/<id>/start` |
| `service.ready` | `action:service/<id>/ready` |
| `finalize` | `action:finalize` |

Edges point **prerequisite → dependent**. Independent ready nodes are ordered
lexicographically by action ID.

A service may `dependsOn` a `migration` task with `completedSuccessfully` so
apply starts that service only after the migrate task succeeds (ADR-009 /
TZ §9.11). `service → postDeploy` remains forbidden. `service → migration`
with `started` or `healthy` is rejected.

## Apply execution (runtime)

The action DAG is the plan; apply journals each node. For `service.start` and
`service.ready`, a failure on one service does **not** stop unrelated later
actions: apply records the diagnostic, skips dependents of that service that
require it, and continues. See [runtime-apply-v1.md](runtime-apply-v1.md).

`service.ready` waits via engine `ps`/`inspect` labels (not
`compose ps --status/--services`). Budget: 2 minutes. Ready requires
`running` and, when the container has a healthcheck, `healthy`.

## Conditions

| Condition | Meaning |
|---|---|
| `started` | Prerequisite service start (or task completion for tasks) |
| `healthy` | Prerequisite `service.ready` (requires healthcheck) |
| `completedSuccessfully` | Prerequisite `task.run` finished successfully |

## Deployment semantic hash

SHA-256 of public runtime semantics (same exclusion of `x-ui` as authoring
semantic hash). Docs-only or `x-ui` edits do not change it.
`oncePerDeployment` tasks key off the hash of command, entrypoint, user, and
mount file bytes (`task-inputs.json`), not this deployment hash.

## Task idempotency limits

The scheduler guarantees phase order, retries/timeouts as declared, and
`oncePerDeployment` skip-when-hash-matches. It does **not** prove that a
migration command is safe to re-run after partial failure; authors must design
tasks accordingly. Default failure policy is `stop`.
