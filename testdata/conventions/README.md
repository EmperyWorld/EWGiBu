# Testdata conventions

- `testdata/golden/` holds stable expected outputs. Environment-specific fields
  (for example `goVersion`) are overwritten in tests before comparison.
- `testdata/manifests/` holds positive and negative authoring fixtures for the
  loader and validate slice.
- `testdata/malicious/` holds path/archive/policy abuse cases; never treat these as
  deployable examples.
- Prefer checked-in bytes over generated temp fixtures when the contract is public.
- Do not store secrets, credentials, private keys, or populated `.env` files.
