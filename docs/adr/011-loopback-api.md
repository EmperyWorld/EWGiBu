# ADR-011: Loopback API, workspace confinement, and CSRF

## Status

Accepted

## Context

The Web UI needs a local authoring API that shares application services with the
CLI. Deployment endpoints must not be exposed. The Go module remains
stdlib-only, so the server uses `net/http` against a checked-in OpenAPI 3.1
contract rather than embedding a foreign OpenAPI runtime.

## Decision

1. **Transport source.** `openapi/v1/openapi.yaml` (OpenAPI 3.1) defines paths,
   schemas, and error shapes. The Go server and TypeScript client must not
   invent alternate DTOs.
2. **Bind.** Default listen address is loopback only (`127.0.0.1` / `::1`).
   Non-loopback binds are rejected.
3. **Workspace/policy.** `--workspace` and optional `--policy` are fixed at
   process start. Requests cannot supply host paths or alternate policy files.
4. **Session/CSRF.** Bootstrap HTML sets an HttpOnly `SameSite=Strict` session
   cookie and embeds a separate CSRF token in a CSP-nonce-protected meta tag.
   Mutations require cookie + `X-Iacpack-CSRF`. Tokens never appear in URLs or
   logs. CORS is disabled.
5. **Saves.** Manifest GET returns ETag (= content SHA-256). PUT requires
   `If-Match` for updates or `If-None-Match: *` to create a missing configured
   file. Save As accepts only workspace-relative paths and refuses overwrite.
6. **Errors.** RFC 9457 Problem Details with a `diagnostics` extension sharing
   CLI codes.
7. **Builds.** In-process queue capacity 2; deterministic job IDs; SSE with a
   1 000-event buffer and `resyncRequired` on overflow. Builds write only under
   an explicit workspace output root. No deploy/apply routes.
8. **Manifest events.** Versioned event kinds:
   `changed|invalid|deleted|permissionDenied|resyncRequired` (poll watcher + SSE).

## Consequences

- CLI and API share `internal/app/*` services for validate/inspect/build/preview.
- Generated/typed clients live under `web/src/api/generated/` and are drift-checked
  against the OpenAPI path list.
- Remote hosted mode and auth services are out of scope.

## Rejected alternatives

- Framework HTTP stacks with foreign module imports — violates module policy.
- Allowing request-selected workspace/policy paths — breaks confinement.
- Cookie-only CSRF (no double-submit header) — weaker against some XSS helpers.
