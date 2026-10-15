# ADR-012: x-ui metadata and semantic hashing

## Status

Accepted

## Context

Authors need canvas positions and viewport state for the visual designer, but
deployment identity must not change when only UI layout moves. Task
`oncePerDeployment` and bundle digests key off deployment semantics, so visual
edits must not force re-runs or false drift.

## Decision

1. `spec.x-ui` is the only v1alpha1 `x-*` extension. Its shape is fixed:
   `nodes` (map of service/task ids → finite `x`/`y`, optional category) and
   optional `viewport` (`x`, `y`, `zoom` in `0.1..4`). Unknown keys under
   `x-ui` or elsewhere fail closed.
2. Semantic hashing (deployment semantic model) **excludes** `spec.x-ui`
   entirely. Position-only or viewport-only edits preserve the hash.
3. `inspect manifest` may surface `x-ui` as authoring metadata but must keep
   canonical public semantics stable for non-UI fields and never include secret
   values.
4. Validation of `x-ui` is lexical/schema only in this phase; graph membership
   of node ids is checked against declared services/tasks.

## Consequences

- UI round-trips can rewrite coordinates without changing lock/bundle digests
  once those phases exist.
- Tests must prove hash immunity for position-only edits.
- Additional `x-*` roots require a new apiVersion or an explicit ADR.

## Rejected alternatives

- Hashing the whole manifest including `x-ui` — couples deploy identity to UI.
- Free-form `x-*` objects — undefinable unknown-field policy.
- Storing UI state only outside the manifest — breaks single-file authoring and
  designer round-trip requirements.
