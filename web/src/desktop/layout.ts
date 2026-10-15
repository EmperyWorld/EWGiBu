import type { WindowId } from "./WindowFrame";

export type WinPos = { x: number; y: number; w: number; h: number };

const PAD = 8;

function clampBox(pos: WinPos, stageW: number, stageH: number): WinPos {
  const w = Math.min(Math.max(48, pos.w), Math.max(48, stageW - PAD * 2));
  const h = Math.min(Math.max(80, pos.h), Math.max(80, stageH - PAD * 2));
  return {
    w,
    h,
    x: Math.min(Math.max(PAD, pos.x), Math.max(PAD, stageW - w - PAD)),
    y: Math.min(Math.max(PAD, pos.y), Math.max(PAD, stageH - h - PAD)),
  };
}

/** Stage-aware defaults so windows stay on-screen, including 320px-wide viewports. */
export function computeWindowDefaults(stageW: number, stageH: number): Record<WindowId, WinPos> {
  const w = Math.max(1, stageW);
  const h = Math.max(1, stageH);
  const narrow = w < 720;

  if (narrow) {
    const iconRail = w < 400 ? 0 : 72;
    const contentW = Math.max(48, w - iconRail - PAD * 2);
    const stackedH = Math.max(120, Math.floor((h - PAD * 3) / 2));
    const designer = clampBox(
      { x: iconRail + PAD, y: PAD, w: contentW, h: Math.min(stackedH + 40, h - PAD * 2) },
      w,
      h,
    );
    const forms = clampBox({ ...designer, y: PAD + 20 }, w, h);
    const preview = clampBox(
      { x: PAD, y: Math.max(PAD, h - stackedH - PAD), w: Math.max(48, w - PAD * 2), h: stackedH },
      w,
      h,
    );
    const diagnostics = clampBox({ ...preview, y: Math.max(PAD, preview.y - 16) }, w, h);
    const inspector = clampBox(
      { x: PAD, y: Math.max(PAD, h - Math.min(180, h / 3) - PAD), w: contentW, h: Math.min(180, h / 3) },
      w,
      h,
    );
    return { designer, forms, preview, diagnostics, inspector };
  }

  const iconRail = 8;
  const sideW = Math.min(280, Math.max(200, Math.floor(w * 0.22)));
  const mainH = Math.max(240, h - PAD * 2);
  const mainW = Math.max(280, w - PAD * 2);

  return {
    designer: clampBox({ x: iconRail, y: PAD, w: mainW, h: mainH }, w, h),
    forms: clampBox({ x: iconRail, y: PAD, w: mainW, h: mainH }, w, h),
    preview: clampBox({ x: Math.max(PAD, w - sideW - PAD), y: PAD, w: sideW, h: Math.floor(mainH * 0.55) }, w, h),
    diagnostics: clampBox(
      { x: Math.max(PAD, w - sideW - PAD), y: PAD + Math.floor(mainH * 0.58), w: sideW, h: Math.floor(mainH * 0.4) },
      w,
      h,
    ),
    inspector: clampBox(
      { x: Math.max(PAD, w - sideW - PAD), y: PAD, w: sideW, h: Math.min(360, mainH) },
      w,
      h,
    ),
  };
}

export function clampWindowPos(pos: WinPos, stageW: number, stageH: number): WinPos {
  return clampBox(pos, stageW, stageH);
}
