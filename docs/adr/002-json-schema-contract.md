# ADR-002: JSON Schema as normative contract

## Status

Accepted

## Context

Authoring manifests, CLI machine output, policies, deployments, and bundles
must share exact public contracts across CLI, API, UI, and documentation.

## Decision

Treat JSON Schema Draft 2020-12 documents under `schemas/` as normative
machine-readable contracts. Go models and loaders validate against these
schemas (plus semantic rules that schemas cannot express). Every CLI
`--output json` command has a checked-in `schemas/cli/v1/<command>.schema.json`.

## Consequences

- Contract changes require simultaneous schema, tests, docs, and ADR updates.
- Golden tests validate JSON against schemas, not only ad-hoc field checks.
- Generated references can be derived from the same schemas later.

## Rejected alternatives

- Go structs alone as the source of truth — opaque to UI/API consumers.
- OpenAPI-only contracts — insufficient for manifests/bundles that are not
  purely HTTP resources.
- Informal Markdown field lists — not machine-testable.
