import { expect, test } from "@playwright/test";

test("release templates propose then confirm write", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("release-templates-open")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("release-templates-open").click();
  await expect(page.getByTestId("release-templates")).toBeVisible();
  await page.getByTestId("release-propose").click();
  await expect(page.getByTestId("release-files")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("release-files")).toContainText("iacpack-ci.yml");
  await page.getByTestId("release-confirm").click();
  await expect(page.getByTestId("release-wrote")).toBeVisible({ timeout: 10_000 });
  await page.getByTestId("release-cancel").click();
});
