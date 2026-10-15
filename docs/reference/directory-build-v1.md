# Directory build — v1

`iacpack build --format directory` materializes a deterministic inspectable
artifact tree. It does not run Docker. `spec.builds` package-manager builds run
in a temporary copy of the context; their output is stored under
`payload/build-<id>/`.

## Layout

```text
manifest.canonical.json
deployment.json
iacpack.lock.json
policy.compile.json
rendered/compose.preview.yaml          # only when deploy params resolve
rendered/preview-unavailable.json      # otherwise
payload/<resource-id>/...
payload/build-<id>/...                 # spec.builds output (index.html and the rest)
builds/<image-id>/...                  # development Dockerfile contexts, not executed here
docs/README.md                         # if package.generateReadme
docs/architecture.mmd
config/.env.example                    # if package.generateEnvExample
```

`.env` is never created. Secret values are never written; Compose preview
references `../secrets/<id>` next to the rendered compose file. Apply writes
those files (mode `0440`) from `--secrets-dir` or `--secret-file` before `compose up` or
`compose run`. A missing file fails before the container starts.

## Package builds

`validate` checks `package.json`, the named script, and the manager lockfile.
It does not run the package manager. `iacpack build` does, only for
`spec.builds`:

- `pnpm install --frozen-lockfile` then `pnpm run <script>` (npm: `npm ci` /
  `npm run`; yarn and bun: `install --frozen-lockfile` then `run`).
- Work happens in a temp copy. `node_modules`, `.git`, and the previous
  `output` directory are not copied. Directories outside `context` that
  `pnpm-workspace.yaml` or a lockfile importer references
  (`../../packages/…`) are copied with that relative path kept. A relative
  import in source that leaves `context` and names an existing workspace file
  is copied the same way. Only `import` / `export` / `require` specifiers are
  followed, not every quoted string. A path that cleans to the workspace root
  is not walked. Symlinks are skipped and are not opened. A path that leaves
  the workspace, or an importer whose directory is missing, is
  `BUILD.FRONTEND` and names that path. The workspace is not modified.
- The pnpm copy is created on the same filesystem as `pnpm store path`. If
  that directory cannot be used, install adds `--package-import-method=copy`
  so a cross-device hardlink is not required. The package-manager failure
  text kept in the diagnostic is the tail of the log, where the non-zero
  exit reason is.
- Frozen install may use the network. The script name is an argv element, not
  a shell string written by iacpack.
- A cache under the state home (`spa-cache`) may reuse a previous non-empty
  output when the copied sources hash matches. An empty or entry-less cache
  entry is not a hit.

## Safety

- Target directory must not exist (no overwrite/replace).
- Staging sibling is verified then renamed; failures leave the target absent.
- Lock-recorded inputs are copied only when hash and size match.
- Source workspace is not modified.
