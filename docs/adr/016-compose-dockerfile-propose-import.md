# ADR-016: Reviewable Compose/Dockerfile propose import

## Status

Accepted

## Context

Operators often already have Docker Compose files and Dockerfiles. iacpack's
authoring format is `iacpack.yaml`; Compose is the rendered/apply artifact, not
the source of truth. Users need an onboarding path that turns existing Compose
(and secondarily Dockerfiles) into a **draft Project** they can review before
Save — without silent mutation of the workspace or build inputs.

P15 forbids new features; this capability is introduced as phase P16.

## Decision

1. **Compose primary, Dockerfile secondary.** Operators propose from a workspace-
   relative Compose path; optional Dockerfile paths add development image stubs.
2. **Propose only.** `import propose` (CLI) and `POST /api/v1/import/propose`
   return `{ project, summary, diagnostics }` and **never write** the workspace.
3. **Accept then Save.** The UI Accept action replaces the in-memory Project
   (undoable). Disk write happens only on explicit Save / Save As.
4. **Subset = inverse of compose-docker/v1 emit** ([`internal/render/compose`](../..))
   plus external networks/volumes. Unsupported or policy-denied fields
   (`privileged`, devices, host networking, Docker socket, `env_file`, unknown
   keys, unsafe binds) become diagnostics and are omitted from the draft.
5. **`${VAR}` → parameter stubs** (no values, no ambient `.env` load). Secret-like
   names are refused rather than imported as literals.
6. **Paths** must be workspace-relative; absolute paths and traversal fail closed.
7. Discover remains suggestions-only (`path` + `kind`). Non-well-known names
   (e.g. `stack9.yml`) are entered via an explicit path field; P16 does not
   widen the discover well-known name set.

## Consequences

- Shared app service used by CLI, API, and UI.
- New public contracts: CLI JSON schema, OpenAPI endpoint, TZ §10/§17 wording.
- Large production Compose trees with host/privileged/`env_file` will import
  partially with explicit refusal diagnostics — by design.

## Rejected alternatives

- Silent Compose→manifest conversion on discover or serve start.
- Full Compose Specification fidelity (profiles, overrides, env_file values).
- Auto-Save after Accept.
- Widening discover name patterns without a separate ADR.

## Amendments

- [ADR-020](020-compose-import-fidelity-layers.md) defines reference layers,
  inverse-emit fidelity targets, diagnostic severities for partial drafts, and
  profile/GPU/path rules. Where ADR-020 clarifies partial import or Accept
  blocking, ADR-020 is normative.
