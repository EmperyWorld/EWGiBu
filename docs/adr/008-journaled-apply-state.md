# ADR-008: Journaled apply, compensation, and instance state

## Status

Accepted

## Context

Runtime must deploy verified bundles on Linux via Docker Compose without
pretending to offer atomic rollback. Concurrent operations on one instance must
be serialized; identity and provider project names must be stable across updates
and independent of local paths. Journal mutation begins in P08; P07 freezes the
contracts.

## Decision

1. **Identity.** Project name comes only from a verified bundle. Instance ID
   defaults to `default` and must match `^[a-z][a-z0-9-]{0,30}$`. Provider
   project slug is:
   `iacp-` + first 40 ASCII chars of `<project>-<instance>` + `-` + first 12
   lowercase hex of SHA-256(`project + NUL + instance`).
2. **State root.**
   `${XDG_STATE_HOME:-$HOME/.local/state}/iacpack/projects/<project>/<instance>/`
   with `state.json`, `journal.db`, `rendered/`, `payload/`, `secrets/`, `logs/`.
   Directories use mode `0700`; secrets files `0440`; logs `0600`. `plan` and
   `doctor` never create persistent state.
3. **Operation lock.** A per-instance exclusive lock file under the state root
   serializes apply/resume/destroy/task. Lock order: verify bundle metadata →
   derive `(project, instance)` → acquire lock → mutate.
4. **State DTO.** Versioned `state.json` (`iacpack.state/v1`) records active
   bundle digest/semantic hash, last run ID, owned resource IDs, and recovery
   hints — never secret bytes. Schema migrations create verified backup
   metadata before rewrite; rollback covers metadata only.
5. **Journal semantics (P08 writes).** Each action transitions
   `planned → started → succeeded|failed|unknown`. Crash inside an action yields
   `unknown` and requires `resume` probe/reconcile. Commit barrier precedes the
   first irreversible task. Compensating cleanup removes only resources created
   by the current run and marked reversible. Named volumes and pre-existing
   resources are never auto-deleted. `service.start` / `service.ready` failures
   are recorded per service and do not abort unrelated later actions
   ([runtime-apply-v1.md](../reference/runtime-apply-v1.md)).
6. **Compensation model.** Resumable deployment with compensating cleanup — not
   atomic rollback. Diagnostics name the exact recovery command.

## Consequences

- Apply-like flows must verify before path/lock derivation.
- Tests cover lock exclusion, identity/slug stability, and plan/doctor zero
  mutation via a recording provider.
- P08 implements journal persistence and mutation methods against these rules.

## Rejected alternatives

- Path-derived project names — unstable across moves/copies.
- Soft locks or shared apply — races and split-brain ownership.
- Claiming transactional rollback across pulls/volumes/migrations — dishonest.
