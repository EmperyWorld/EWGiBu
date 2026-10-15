# ADR-019: GitHub-oriented release templates without auto-publish

## Status

Accepted

## Context

Operators use iacpack to prepare a workspace for publication on GitHub. TZ.md
forbids automatic publication from the application. What is missing is
**checked-in, reviewable packaging guidance**: CI workflow templates, ignore
files, and docs that wire validate/lock/build/verify into a human-controlled
release — not a "Push to GitHub" button.

## Decision

1. **Template pack (authoring, explicit write):** `iacpack release init-templates`
   (name fixed in P19) writes **only** documented files the operator opts into,
   under the workspace, refusing overwrite unless `--force` with confirmation
   semantics matching other authoring commands. Default set:
   - `.github/workflows/iacpack-ci.yml` (pinned actions; validate/lock/build/verify;
     no secrets in logs; no publish job by default)
   - `.gitignore` / `.gitattributes` snippets for iacpack outputs (or append
     markers)
   - `docs/iacpack-release.md` operator checklist (human publish steps)
2. **No network publish:** templates never call `gh`, never push, never create
   remotes, never upload artifacts to registries from the app.
3. **UI:** "Release templates…" propose preview of files to write → Confirm
   writes via API equivalent to CLI; still loopback-only.
4. **Examples:** `examples/release-templates/` holds golden template bodies used
   by the command (deterministic bytes).
5. **Human publish remains external:** signing identity, tag, and GitHub release
   stay operator-controlled as in P15.

## Consequences

- New authoring command + UI + schemas; expands "GitHub-ready" without violating
  the no-auto-publish invariant.
- CI consumers get a known-good starting workflow with pinned digests/versions.

## Rejected alternatives

- In-app GitHub OAuth / `gh release create` / auto-push.
- Telemetry or hosted release service.
- Unpinned `actions/checkout@main`-style templates.
