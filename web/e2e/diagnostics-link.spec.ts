import { expect, test } from "@playwright/test";

test("unmodeled JSON fallback round-trips on a service", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("service-app")).toBeVisible({ timeout: 15_000 });

  const box = page.getByTestId("service-unmodeled-app");
  await box.fill('{"xCustom":"from-ui"}');
  await box.blur();
  await expect(page.getByTestId("status")).toContainText(/unsaved/i);
  await expect(box).toHaveValue(/xCustom/);
});

test("field-path anchors exist for diagnostic navigation targets", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.locator('[data-field-path="/metadata/name"]')).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('[data-field-path="/spec/services/app"]')).toBeVisible();
  await expect(page.locator('[data-field-path="/spec/services/app/ports"]')).toBeVisible();
});

test("save with empty name validates, raises diagnostics, and focuses field", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  const name = page.getByTestId("meta-name");
  await expect(name).toBeVisible({ timeout: 15_000 });
  await name.fill("");
  await expect(page.getByTestId("status")).toContainText(/unsaved/i);

  await page.getByTestId("save").click();
  await expect(page.getByTestId("status")).toContainText(/issue|validation|lock/i, {
    timeout: 20_000,
  });
  await expect(page.getByTestId("diagnostics-list")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("diagnostics-list")).toContainText(/non-empty|name/i);

  // Auto-navigate focuses the offending field; link remains usable.
  await expect(name).toBeFocused({ timeout: 10_000 });
  await page.getByTestId("diag-field-0").click();
  await expect(name).toBeFocused();
  await expect(page.locator('[data-field-path="/metadata/name"]')).toHaveClass(/field-flash/);
});
