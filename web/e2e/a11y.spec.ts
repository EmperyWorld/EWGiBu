import { expect, test } from "@playwright/test";

test("landmarks, skip link, and status live region", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("banner")).toBeVisible();
  await expect(page.getByRole("main")).toBeVisible();
  await expect(page.getByRole("toolbar", { name: "Authoring actions" })).toBeVisible();
  await expect(page.getByTestId("status")).toHaveAttribute("aria-live", "polite");

  await page.keyboard.press("Tab");
  const skip = page.getByRole("link", { name: "Skip to main content" });
  await expect(skip).toBeFocused();
  await skip.press("Enter");
  await expect(page.locator("#main-content")).toBeFocused();
});

test("tabs are keyboard operable", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("tab-designer")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("tab-designer").focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByTestId("tab-forms")).toBeFocused();
  await expect(page.getByTestId("tab-forms")).toHaveAttribute("aria-selected", "true");
});

test("status conveys sync meaning with text not color alone", async ({ page }) => {
  await page.goto("/");
  const status = page.getByTestId("status");
  await expect(status).toBeVisible({ timeout: 15_000 });
  const color = await status.evaluate((el) => getComputedStyle(el).color);
  expect(color).toBeTruthy();
  // Toolbar status is never empty after load and includes textual cues when dirty.
  await page.getByTestId("tab-forms").click();
  await page.getByTestId("meta-name").fill("a11y-dirty");
  await expect(status).toContainText(/unsaved/i);
});

test("graph has textual dependency alternative", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-designer").click();
  await page.getByTestId("dep-drawer").locator("summary").click();
  await expect(page.getByRole("table")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("table")).toContainText(/app|api/i);
});

test("reduced motion disables dependency edge animation", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await page.getByTestId("tab-designer").click();
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });
  const animated = page.locator(".react-flow__edge.animated");
  await expect(animated).toHaveCount(0);
});

test("320px viewport keeps designer window on stage", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto("/");
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });
  const stage = page.locator("#main-content");
  const win = page.locator('[data-window="designer"]');
  await expect(win).toBeVisible();
  const box = await Promise.all([stage.boundingBox(), win.boundingBox()]);
  expect(box[0] && box[1]).toBeTruthy();
  const s = box[0]!;
  const w = box[1]!;
  expect(w.x).toBeGreaterThanOrEqual(s.x - 1);
  expect(w.y).toBeGreaterThanOrEqual(s.y - 1);
  expect(w.x + w.width).toBeLessThanOrEqual(s.x + s.width + 2);
  expect(w.y + w.height).toBeLessThanOrEqual(s.y + s.height + 2);
  await expect(page.getByRole("toolbar", { name: "Authoring actions" })).toBeVisible();
});
