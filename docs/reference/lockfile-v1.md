# Lockfile reference — `iacpack.lock/v1`

Normative schema: [`schemas/lock/v1/schema.json`](../../schemas/lock/v1/schema.json)  
(`$id`: `iacpack.schemas.lock.v1`). Serialization: RFC 8785 canonical JSON + trailing newline.

## Generation

`iacpack lock` is the only command that writes `iacpack.lock.json`. It:

1. Validates the manifest (schema, semantics, compile policy).
2. Resolves explicit build parameters (`--set` > `--values` > defaults; never ambient env / `.env`).
3. Expands declared `spec.resources` and image `build` include/exclude globs.
4. Records digest-pinned OCI refs (mutable tags require explicit `--update-images`).
5. Atomically replaces the lockfile (prior bytes preserved on failure).

`validate --locked` compares a freshly computed lock document to the on-disk file and fails on drift.

## Doublestar globs

Include/exclude patterns use `/` only:

| Token | Meaning |
|---|---|
| `*` | Any characters within one path segment |
| `?` | One character within one path segment |
| `**` | Zero or more full path segments (must be a whole segment) |

Matches are sorted by UTF-8 path bytes. Empty matches fail unless `allowEmpty: true`.

## Path safety

Workspace paths reject absolute forms, `..`, backslashes, drive/UNC prefixes, NUL, and combining characters (force Unicode NFC). Devices, sockets, and FIFOs are rejected. A hardlink is stored as a normal file. A symlink is accepted only when its target stays inside the workspace. The same workspace path declared by more than one resource is recorded once. Case-folding collisions of distinct paths are errors. Include patterns limit the walk. A file source is that file, not a directory walk. A manifest in `<workspace>/imports/` uses the parent directory as the workspace root. Output paths must not be nested inside declared inputs.

## Excluded from lockfile

Timestamps, username, hostname, absolute workspace paths, secret values, and ambient environment.
