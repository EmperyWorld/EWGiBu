# iacpack web UI (P13)

Pinned toolchain: **Node.js 24.21.0** / **pnpm 10.34.6** / **Vite 8.3.1** /
**React 19.3.0** / **@xyflow/react 12.12.0** / **openapi-typescript 7.13.0** /
**TypeScript 7.0.2** (`tsc`) with **TypeScript 6.0.3** for `typescript-eslint`
(ESLint has no TypeScript 7 programmatic API yet).

```bash
pnpm install
pnpm check        # lint, typecheck, unit tests, OpenAPI drift
pnpm build        # production assets -> dist/
../scripts/sync-ui-assets.sh   # copy into Go embed tree
pnpm test:e2e     # Playwright (requires make build + embedded UI)
```

From repo root: `make test-ui` runs the full UI gate and syncs embed assets.
