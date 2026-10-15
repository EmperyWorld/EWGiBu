# CLI reference (v1 surface)

Полное руководство: [GUIDE.md](GUIDE.md). Пошаговый прогон: [CONSOLE.md](CONSOLE.md).

Machine envelopes: `schemas/cli/v1/*.schema.json` with `--output json`.

| Command | Purpose |
|---------|---------|
| `version` | Product/version identity |
| `init --output <path>` | Create minimal manifest (no overwrite) |
| `validate --file <path> [--locked]` | Schema/semantic/input/policy (read-only) |
| `lock --file <path>` | Write `iacpack.lock.json` |
| `build --file --format directory\|bundle --output` | Artifact. `spec.builds` runs the declared package manager in a temp copy and stores `payload/build-<id>/` |
| `verify <artifact>` | Bundle integrity |
| `render compose` | Compose preview render |
| `plan <bundle-or-kit>` | Plan with `--values` and `--secrets-dir` (or `--secret-file`). A kit directory is checked, then planned from `project.iacbundle`. Stops with `lock устарел` when the workspace draft is newer than the bundle lock |
| `apply <bundle-or-kit>` | Journaled apply. Before state writes, reports `APPLY.IMAGE_PULL`, and stops with exit 6 on `APPLY.IMAGE_LOCAL`, `APPLY.HOST_PATH`, `SECRET.MISSING`, or `PARAM.MISSING`. A kit directory is checked first, then `project.iacbundle` starts. Unhealthy services print the last 80 log lines ([runtime-apply-v1](../reference/runtime-apply-v1.md)) |
| `bundle --workspace --output-dir [--policy] [--embed-local-images]` | Portable kit plus `project.iacbundle`. Empty host paths and unembedded local images are warnings; `plan`/`apply` of the kit are the gate |
| `explain <bundle> <service>` | Image, gpu, `ldLibraryPath`, devices |
| `resume` / `status` | Recovery / instance status |
| `destroy` | Tear down (`--delete-volumes` explicit) |
| `doctor` | Environment diagnostics |
| `images fetch` | Offline image materialization helpers |
| `manifest migrate` | Explicit manifest migration |
| `inspect …` | Manifest/graph/inputs/bundle/discover |
| `schema export` | Emit embedded schemas |
| `serve --workspace <dir>` | Loopback API + UI |

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success, including a no-op apply and `bundle` warnings |
| 2 | Usage |
| 3 | Invalid manifest or check |
| 4 | I/O |
| 5 | Bundle unreadable |
| 6 | Stop before mutation: local image, host path, secret, deploy parameter, GPU, stale lock, kit preflight |
| 7 | Mutation already happened; re-apply resumes, there is no rollback |
| 70 | Internal |

## Flags used by the operator flows

```text
iacpack import propose --workspace <path> [--compose <rel> ...] [--dockerfile <rel> ...] [--profile <name> ...] [--import-strict] [--name <id>] [--existing-project-file <json>] [--output text|json]
iacpack bundle --workspace <dir> --output-dir <dir> [--policy <path>] [--embed-local-images] [--output text|json]
iacpack plan <bundle-or-kit> [--instance <id>] [--workspace <dir>] [--set k=v] [--values <path>] [--secrets-dir <dir>] [--secret-file id=path] [--operator-policy <path>] [--output text|json]
iacpack apply <bundle-or-kit> [--instance <id>] [--workspace <dir>] [--set k=v] [--values <path>] [--secrets-dir <dir>] [--secret-file id=path] [--operator-policy <path>] [--container-engine podman|docker] [--docker-socket <path>] [--yes] [--output text|json]
iacpack explain <bundle> <service> [--output text|json]
```

Global contracts: no secret values on argv; `validate` does not run project code or the network; `build` runs a package manager only for `spec.builds` (frozen install may use the network); paths confined.
