import { expect, test } from "@playwright/test";

test("forms Add creates entities without window.prompt", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-services")).toBeVisible({ timeout: 15_000 });

  await page.getByTestId("map-services-add-id").fill("worker");
  await page.getByTestId("map-services-add").click();
  await expect(page.getByTestId("service-worker")).toBeVisible();
  await expect(page.getByTestId("status")).toContainText(/Local|unsaved|dirty/i);

  await page.getByTestId("map-networks-add-id").fill("backend");
  await page.getByTestId("map-networks-add").click();
  await expect(page.getByTestId("map-networks")).toContainText("backend");

  await page.getByTestId("map-parameters-add-id").fill("Bad_Id");
  await page.getByTestId("map-parameters-add").click();
  await expect(page.getByTestId("map-parameters-add-error")).toContainText(/kebab-case/i);
});
