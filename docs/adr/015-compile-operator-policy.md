# ADR-015: Compile policy and operator policy layering

## Status

Accepted

## Context

Packaging needs a checked-in, digestable compile policy (registries, development
builds, external references, ceilings/quotas) while operators must be able to
tighten limits at plan/apply without rewriting the bundle. Unsupported
privileged capabilities must stay impossible even if a policy file tries to
enable them.

## Decision

1. Compile policy uses schema `iacpack.schemas.policy.v1` (`iacpack.policy/v1`
   document kind). An immutable default is checked in under `policies/` and
   embedded for pure evaluation during `validate`/`lock`/`build`.
2. Default compile policy (TZ.md §9.15 / §13.2):
   - registries: `docker.io`, `ghcr.io`, `quay.io`;
   - development builds only when manifest `mode` is `development`;
   - build network off;
   - external networks/volumes off;
   - package quotas/ceilings at §13.2 defaults (policy may only lower them).
3. Explicit `--policy` may widen only capabilities that exist in the v1
   schema/provider (registry allowlist, development builds, build network,
   external refs, lower quotas). It cannot invent privileged/host/device/socket
   options absent from the manifest schema.
4. Operator policy is a separate, local plan/apply input that may only be
   stricter than the effective compile policy. It is never trusted from the
   bundle. Bundle may declare required capabilities; operator policy always
   wins for denial.
5. Policy evaluation in P02 is pure (no filesystem/network). Unsupported
   capability keys in a policy document fail closed.

## Consequences

- Lock/bundle will record the effective compile policy digest (later phases).
- High-risk allowances are explicit and appear in plan/bundle metadata later.
- Tests cover default deny of external refs and build-network, and registry
  allowlist enforcement on image refs.

## Rejected alternatives

- Single mutable global policy file with silent defaults — non-reproducible.
- Allowing policy to enable schema-unsupported privileged flags — unsafe.
- Embedding operator policy in the bundle as authoritative — operators lose
  control and offline trust model breaks.
