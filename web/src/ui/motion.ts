/** Respect OS “reduce motion” for scroll, flash, and canvas camera. */

export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function scrollBehavior(): ScrollBehavior {
  return prefersReducedMotion() ? "auto" : "smooth";
}

export function fitViewDuration(): number {
  return prefersReducedMotion() ? 0 : 200;
}

export function fieldFlashMs(): number {
  return prefersReducedMotion() ? 0 : 1200;
}
