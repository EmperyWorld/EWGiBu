# Web UI accessibility checklist (WCAG 2.2 AA)

Automated coverage: Playwright `e2e/a11y.spec.ts` (landmarks, skip link, status
live region, keyboard tab switching, contrast smoke via computed styles).

Manual checks (run before release; record date/operator in PR notes):

1. **Keyboard only** — Tab/Shift+Tab through toolbar, tabs, forms, graph controls,
   dependency table Disconnect, preview/focusable compose. No keyboard trap.
2. **Focus order / restoration** — After Save conflict / external-change banner
   appears, focus moves into the banner recovery actions (`sync-banner`); dismissing
   recovery (reload/discard) restores the previously focused control when still mounted.
   Covered by `e2e/sync.spec.ts` and `SyncBanner.test.tsx`.
3. **Headings / landmarks** — One `h1`, section `h2`s, `banner`/`main`/`complementary`
   (aside), skip link works.
4. **Forms / errors** — Labels associated with controls; diagnostics list readable
   by screen reader; errors not color-only (severity text).
5. **Graph textual alternative** — Dependency table lists edges; property panel
   announces selection via `aria-live`.
6. **Contrast** — Accent on panel (~teal on cream) and error/warn text readable at
   AA; focus ring (`--focus`) visible.
7. **Reduced motion** — OS “reduce motion” disables animated edges, fit-view
   camera tween, diagnostic field flash, and smooth scroll. Automated:
   `e2e/a11y.spec.ts` (edge animation) and `src/ui/motion.test.ts`.

8. **Zoom / reflow** — 200% and 320px-wide viewport: toolbar wraps, windows stay
   inside `#main-content` (no forced 640px floor). Automated: `e2e/a11y.spec.ts`
   (320px) and `src/desktop/layout.test.ts`.

9. **Status announcements** — Dirty/save/sync strings in `#status` (`aria-live=polite`)
   and sync banner (`role=alert`).
10. **Pointer alternatives** — Connect/Disconnect and forms cover graph drag gestures.

Non-goals: third-party React Flow chrome may expose extra unlabeled widgets; prefer
toolbar Connect/Disconnect and the dependency table for AT paths.

## Sign-off

| Release | Date | Automated a11y | Manual checklist | Notes |
|---------|------|----------------|------------------|-------|
| 1.1.0 | 2026-09-29 | pass (`e2e/a11y.spec.ts`) | reviewed | Residual Flow chrome accepted |
