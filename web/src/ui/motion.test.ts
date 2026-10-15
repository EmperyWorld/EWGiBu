import { afterEach, describe, expect, it, vi } from "vitest";
import { fieldFlashMs, fitViewDuration, prefersReducedMotion, scrollBehavior } from "./motion";

describe("motion helpers", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("defaults to motion when matchMedia is unavailable", () => {
    vi.stubGlobal("window", { matchMedia: undefined });
    expect(prefersReducedMotion()).toBe(false);
    expect(scrollBehavior()).toBe("smooth");
    expect(fitViewDuration()).toBe(200);
    expect(fieldFlashMs()).toBe(1200);
  });

  it("disables animation when OS reduce-motion is set", () => {
    vi.stubGlobal("window", {
      matchMedia: (q: string) => ({
        matches: q.includes("prefers-reduced-motion") && q.includes("reduce"),
        media: q,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    });
    expect(prefersReducedMotion()).toBe(true);
    expect(scrollBehavior()).toBe("auto");
    expect(fitViewDuration()).toBe(0);
    expect(fieldFlashMs()).toBe(0);
  });
});
