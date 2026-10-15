# ADR-018: Secrets and parameters onboarding from import

## Status

Accepted

## Context

Imported Compose often references `${VAR}`, `env_file`, and secret-like keys.
P16 maps safe interpolations to parameter stubs and omits secret-like / env_file
content. Operators still need a guided path to declare **secret references** and
**deploy/build parameters** without ever pasting secret values into manifests,
lockfiles, UI state, or diagnostics (ADR-005).

## Decision

1. **Parameter onboarding panel:** after propose/Accept (or from Forms), list
   unresolved parameters (no default) with phase/type hints; operator may set
   non-secret defaults or leave required-without-default.
2. **Secret declaration stubs:** when import sees `env_file`, secret-like env
   keys, or Compose `secrets:` mounts, emit **declarations only**
   (`spec.secrets.<id>`). A secret-like environment key stays in
   `environment` as `{secret: id}`. The id comes from the variable inside
   `${…}`, not from the key name, so `${POSTGRES_PASSWORD}` and
   `${KEYCLOAK_DB_PASSWORD}` are different secrets. Never read file contents;
   never put values in draft YAML. Numeric settings such as
   `JITSI_TOKEN_TTL_SECONDS` stay parameters.
3. **env_file handling:** treat each listed env_file path as a refusal for
   *values*. The secret id is derived from the path, so distinct files do not
   share one id. File contents are not read.
4. **`.env.example` only:** import/onboarding may refresh/generate
   `.env.example` placeholders for declared parameters/secrets; MUST NOT create
   or package a populated `.env`.
5. **UI/CLI:** `import secrets-propose` (or extend propose result with
   `secretSuggestions[]` / `parameterGaps[]`) remains propose-only; Apply to
   in-memory Project is explicit; Save is explicit.

## Consequences

- Aligns onboarding with ADR-005 and secret canaries.
- Improves post-import authoring without weakening fail-closed policy.
- New CLI/API fields and UI panel; schemas/goldens required.

## Rejected alternatives

- Loading `.env` / `env_file` values into parameters or environment literals.
- Storing secret values in browser session, SSE, or diagnostics.
- Auto-Save of secret stubs without review.
