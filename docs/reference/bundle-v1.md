# Bundle format v1 (`.iacbundle`)

Deterministic gzip-compressed USTAR archive. Extension: `.iacbundle`.

## Layout

```text
bundle.json
manifest.canonical.json
deployment.json
iacpack.lock.json
policy.compile.json
rendered/compose.preview.yaml   # or rendered/preview-unavailable.json
payload/<resource-id>/...
payload/build-<id>/...         # spec.builds output, when declared
docs/README.md
docs/architecture.mmd
config/.env.example
```

`bundle.json` lists path/size/mode/SHA-256 for every other entry and does not
hash itself. Offline OCI layouts (`images/oci-layout/...`) are reserved for a
later phase.

## Determinism constants

| Field | Value |
|---|---|
| Tar format | POSIX USTAR |
| Path encoding | UTF-8; combining characters rejected (NFC required) |
| Entry order | `bundle.json` first, then remaining files by UTF-8 path |
| mtime | Unix epoch |
| UID/GID | 0; empty uname/gname |
| Directory mode | `0755` |
| Regular file mode | `0644`, or `0755` when the workspace file has an execute bit or the resource declares mode `0755` |
| Gzip | DEFLATE level 9; mtime epoch; OS 255; empty Name/Comment/Extra |
| Text | UTF-8, LF, no BOM |

## Verifier quotas (defaults)

- 50 000 entries
- 4 GiB per entry
- 20 GiB expanded total
- compression ratio 200:1
- path ≤ 255 UTF-8 bytes

Hard caps (not overridable by policy): 100 000 entries, 100 GiB expanded.

## Compatibility

- `formatVersion` must be `iacpack.bundle/v1`.
- Unsupported versions are rejected; silent migration is forbidden.
- Signatures and image embedding are out of scope for P06.

## Commands

```text
iacpack build --format bundle --file … --output out.iacbundle
iacpack verify out.iacbundle
iacpack inspect bundle out.iacbundle
iacpack render compose out.iacbundle [--set k=v]
iacpack diff --from a.iacbundle --to b.iacbundle
```
