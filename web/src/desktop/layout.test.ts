import { describe, expect, it } from "vitest";
import { clampWindowPos, computeWindowDefaults } from "./layout";

function onStage(pos: { x: number; y: number; w: number; h: number }, sw: number, sh: number) {
  expect(pos.x).toBeGreaterThanOrEqual(0);
  expect(pos.y).toBeGreaterThanOrEqual(0);
  expect(pos.x + pos.w).toBeLessThanOrEqual(sw + 0.5);
  expect(pos.y + pos.h).toBeLessThanOrEqual(sh + 0.5);
}

describe("computeWindowDefaults", () => {
  it("keeps every window inside a 320x640 stage", () => {
    const d = computeWindowDefaults(320, 640);
    for (const pos of Object.values(d)) onStage(pos, 320, 640);
    expect(d.designer.w).toBeLessThanOrEqual(320 - 16);
  });

  it("keeps every window inside a 1280x720 laptop stage", () => {
    const d = computeWindowDefaults(1280, 720);
    for (const pos of Object.values(d)) onStage(pos, 1280, 720);
    expect(d.designer.w).toBeGreaterThan(700);
    expect(d.designer.x).toBeLessThan(24);
    expect(d.designer.h).toBeGreaterThan(600);
  });

  it("clampWindowPos never exceeds the stage", () => {
    const c = clampWindowPos({ x: 900, y: 900, w: 800, h: 800 }, 320, 500);
    onStage(c, 320, 500);
  });
});
