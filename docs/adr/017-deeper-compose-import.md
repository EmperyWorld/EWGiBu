# ADR-017: Deeper Compose import and multi-file onboarding

## Status

Accepted

## Context

P16 (ADR-016) delivers a single-file Compose subset propose→Accept→Save path.
Real operator trees use many Compose-like filenames (`stackN.yml`,
`compose.*.yml`), multiple files, and dense refusal diagnostics. Without deeper
onboarding, Accept produces drafts that are too incomplete for daily use.

## Decision

1. **Widen discover (suggestions only)** to include deterministic patterns for
   Compose-like YAML under depth ≤ 3: well-known names from P03 **plus**
   `compose.*.yml|yaml`, `docker-compose.*.yml|yaml`, and `stack*.yml|yaml`.
   Still skip hidden dirs, output roots, `.env`, and secret-like files. Never
   mutate build inputs from discover alone.
2. **Multi-select propose:** `import propose` accepts multiple Compose paths
   (ordered, deterministic merge). Later files may not silently overwrite
   earlier entity ids; collisions become diagnostics with stable resolution
   rules (first-wins + rename suggestion).
3. **Richer refusal report:** propose result includes a structured `refusals[]`
   (path, pointer, code, category: policy|unsupported|secret|path) in addition
   to diagnostics, for UI grouping.
4. **Import UX:** UI lists discover hits by kind, multi-select Compose +
   Dockerfiles, shows refusal categories, and keeps Accept→Save semantics from
   ADR-016.
5. **Still not full Compose Spec:** no profiles/overrides merge as apply
   semantics; no ambient `.env` values; policy denials remain fail-visible.

## Consequences

- Public discover suggestion set changes (contract + goldens).
- Import CLI/API request/response schemas expand.
- Large stacks become reviewable with less manual path typing.

## Rejected alternatives

- Silent auto-merge of all Compose files on `serve` start.
- Full Compose override/profile fidelity.
- Reading `.env` to fill parameter defaults.

## Amendments

- [ADR-020](020-compose-import-fidelity-layers.md) specifies `activeProfiles`
  on propose, partial `propose.ok` semantics, and the operator-facing fidelity
  matrix ([`compose-import-fidelity.md`](../reference/compose-import-fidelity.md)).
