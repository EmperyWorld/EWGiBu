# Declared-input glob semantics

Used by `spec.resources.*.include|exclude` and `spec.images.*.build.include|exclude`.

See also [`lockfile-v1.md`](./lockfile-v1.md) and `TZ.md` §10.2.

## Rules

1. Patterns and matched names are slash-separated (`/`). Backslash is rejected.
2. `*` and `?` never cross `/`.
3. `**` is a whole segment and matches zero or more segments.
4. Character classes (`[...]`) and brace expansion (`{a,b}`) are not supported in v1.
5. Matching is against the path relative to the resource `source` or image `context`.
6. Results are sorted lexicographically by UTF-8 bytes before hashing/locking.
