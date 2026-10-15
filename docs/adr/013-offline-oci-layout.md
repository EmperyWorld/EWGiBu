# ADR-013: Offline OCI image cache and embed

## Status

Accepted

## Context

Offline distributions must carry digest-pinned images without registry access
at apply time. Fetching is an explicit network-enabled command; build/embed
must remain network-free. Runnable code cannot import `go-containerregistry`.

## Decision

1. **`iacpack images fetch`** reads a lockfile, pulls only locked digests via a
   `Fetcher` adapter (default: `docker pull` + `docker save` argv arrays with
   sanitized env; tests use an in-memory/fixture fetcher), and writes a
   deterministic OCI image-layout cache under `--output`.
2. **Cache layout.** `image-cache/oci-layout`, `index.json`, and
   `blobs/sha256/<hex>` with content addressed by digest. Blobs are
   deduplicated. Paths and JSON are sorted/canonical.
3. **`build --embed-images --image-cache <dir>`** is network-free: every
   descriptor/blob must match lock digests; content is copied into
   `images/oci-layout/` inside the artifact/bundle. Missing/corrupt/swapped
   blobs fail closed.
4. **Apply offline.** When the bundle declares `embedImages`, apply imports
   from the embedded layout (`docker load` / layout import) and must not
   perform registry GETs. Online bundles keep pull-by-digest behavior.
5. **Quotas.** OCI blobs count toward bundle entry/expanded quotas; hard caps
   remain unoverridable.

## Consequences

- Docker Engine (or a test fetcher) is required for fetch, not for verify of an
  already-built offline bundle.
- Golden tests cover multi-arch index fixtures and blob tampering.

## Rejected alternatives

- Silent registry access during `build` — violates pure-compiler rules.
- Vendoring containerregistry — foreign module paths forbidden in product code.
- Non-content-addressed tar of `docker save` without digest verification —
  unsafe for offline trust.
