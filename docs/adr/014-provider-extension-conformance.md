# ADR-014: Provider extension and conformance process

## Status

Accepted

## Context

Future runtimes may be desirable, but ad-hoc providers would fork semantics for
plans, secrets, health, and destroy behavior.

## Decision

Additional provider profiles are allowed only through an explicit extension
process: ADR, schema/capability declarations, security threat model, golden
fixtures, and a conformance suite covering validate/build/plan/apply/health/
resume/destroy invariants. Until conformance passes, a provider must not ship
in-tree or be selectable from authoring manifests.

## Consequences

- `compose-docker/v1` remains the only supported profile until another clears
  the gate.
- Capability flags and policy layering stay provider-aware but schema-driven.
- Marketing names cannot bypass conformance.

## Rejected alternatives

- Open plugin ABI in v1 without conformance — unpredictable security posture.
- Soft “experimental” providers in release builds — user-facing footguns.
- Translating Compose output to other orchestrators implicitly — false
  equivalence.
