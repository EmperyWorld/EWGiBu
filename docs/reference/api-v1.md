# Local authoring API v1

Transport source: [`openapi/v1/openapi.yaml`](../../openapi/v1/openapi.yaml) (OpenAPI 3.1).  
ADR: [011-loopback-api.md](../adr/011-loopback-api.md).

## Serve

```bash
iacpack serve --workspace <dir> [--addr 127.0.0.1:7432] [--manifest iacpack.yaml] [--policy <path>] [--output-root .iacpack-out]
```

- Bind is loopback-only (`127.0.0.1` / `::1`).
- Workspace and compile policy are fixed at process start.
- Placeholder HTML at `/` bootstraps an HttpOnly `SameSite=Strict` session cookie
  and a CSRF token in a CSP-nonce-protected meta tag. Mutations require
  `X-Iacpack-CSRF`. CORS is disabled.

## Endpoints

Health, schema, manifest GET/PUT (ETag / `If-Match` / `If-None-Match: *`), Save As,
canonical export, validate, graph, compose/artifact preview, lock, builds
(start/status/cancel/SSE), manifest events (poll watcher + SSE; see
[ui-sync-v1.md](./ui-sync-v1.md)), inputs, discover (read-only workspace suggestions;
same rules as `inspect --discover`). `POST /api/v1/secrets/generate` writes
`secrets/<id>` in the workspace and returns the path only, never the file bytes.
The Web UI refreshes the lockfile after a successful Save so preview/build stay aligned.

No deploy/apply/status/destroy routes.

## Client

Generated TypeScript client: `web/src/api/generated/`. Drift is checked by Go tests
and `pnpm check` in `web/`.
