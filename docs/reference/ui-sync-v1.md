# UI disk synchronization (v1.1)

Authoring UI ↔ workspace manifest awareness. Transport: `GET /api/v1/manifest/events`
(SSE). Contract: [`openapi/v1/openapi.yaml`](../../openapi/v1/openapi.yaml) `ManifestEvent`.
ADR: [011-loopback-api.md](../adr/011-loopback-api.md).

## Guarantees

- Keystrokes never write the configured manifest. Only explicit Save / Save As /
  recover-overwrite write.
- Save uses ETag / `If-Match` (create-missing requires `If-None-Match: *`).
- A watcher SSE event is **not** write permission.
- Valid external edits are announced without a page reload. Clean UI may reload;
  dirty UI enters **conflict** (no silent merge).
- Invalid external YAML stays on disk. The UI retains the last valid visual model,
  shows diagnostics, and disables Save until the user arms overwrite or discards.
- Self-saves are suppressed so UI Save does not echo as an external `changed` event.
- Canonical Save may normalize YAML formatting/comments; deployment semantics and
  `x-ui` are preserved per schema (ADR-012).

## Event kinds

| kind | Meaning |
|------|---------|
| `changed` | Readable, looks like Project YAML, bytes/revision changed |
| `invalid` | Bytes changed but not a recognizable Project YAML |
| `deleted` | Configured path missing |
| `permissionDenied` | Unreadable / symlink / access error |
| `resyncRequired` | Buffer overflow, reconnect miss, or bootstrap |

Payload is `{kind, revision}` only — never file content. SSE `id` supports
`Last-Event-ID` replay from a bounded (≤1000) in-memory buffer; overflow prepends
`resyncRequired`.

## Recovery choices

| Situation | UI actions |
|-----------|------------|
| Clean + external change | Reload from disk |
| Dirty + external / resync | Export local YAML, Discard local, or Arm overwrite → Overwrite disk |
| Invalid / deleted disk | Export local, Arm overwrite → Overwrite (or recreate with `If-None-Match: *`) |
| Permission denied | Fix OS permissions externally; Save stays disabled |
| Stale Save (HTTP 409) | Treated as conflict; same recovery set |

## Limitations (non-goals)

- No CRDT / collaborative merge / cloud sync.
- Comments are not guaranteed to survive canonical Save.
- Poll-based watcher (stdlib) — debounce ~350ms; not instant on every platform.
- Two browser tabs are independent sessions; each must resolve its own conflict.
- No remote/non-loopback serve mode in v1.1.

## Accessibility status strip

Status is announced with text (not color alone): unsaved / saving / saved /
external change / conflict / invalid disk / deleted / permission denied /
sync reconnecting.
