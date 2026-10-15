# ADR-005: Secret references and file delivery

## Status

Accepted

## Context

Deployment needs secrets, but manifests, lockfiles, bundles, logs, and
diagnostics must never contain secret values.

## Decision

Manifests declare secret *references* only. Delivery mode for v1 is
`delivery: file`: the operator provides secret files at deploy time. Runtime
mounts those files read-only (`0440` under a `0700` directory). Secret values
are forbidden in manifests, lockfiles, packages, Compose environment dumps,
diagnostics, process arguments, and journals.

## Consequences

- `.env.example` may list names/placeholders only.
- Plan/apply output must mask or omit secret contents. Operators pass
  `--secrets-dir` (file name is the id) or `--secret-file id=path`. An explicit
  pair overrides the same id from the directory.
- UI/API never return secret values to the browser.
- Authoring may write a generated file at workspace `secrets/<id>` (`password`,
  `port`, or `name`). The HTTP response carries the id and relative path only.
  An existing file is replaced only when the operator confirms.
- Argv and concatenated env values may name a secret id (`{secret}` or
  `{parts}`). Render writes `${IACPACK_SECRET_<ID>}`, not the secret bytes.
  Apply injects the value from the secret file into the Compose process
  environment. Compose file mounts use `../secrets/<id>` under the instance
  state root (mode `0440`). A missing file fails before the container starts.
  A numeric service or task uid other than `0` is `chown`ed onto that file
  before start so it can read mode `0440`.

## Rejected alternatives

- Inline secret values in YAML — inevitable leakage via VCS and bundles.
- Environment-variable-only delivery as the sole mode — harder to audit file
  permissions and easier to leak via process listings.
- Encrypting secrets into the bundle — expands key-management scope beyond v1.
