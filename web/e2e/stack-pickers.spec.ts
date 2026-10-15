import { expect, test } from "@playwright/test";

test("image build picks Dockerfile from workspace discover", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-images")).toBeVisible({ timeout: 15_000 });

  // Local Dockerfile builds require development mode.
  await page.getByTestId("mode").selectOption("development");

  await page.getByTestId("map-images-add-id").fill("local-app");
  await page.getByTestId("map-images-add").click();
  await expect(page.getByTestId("image-local-app")).toBeVisible();
  await expect(page.getByTestId("image-kind-local-app")).toHaveValue("build");

  await expect(page.getByTestId("image-dockerfile-pick-local-app")).toBeEnabled({ timeout: 10_000 });
  await page.getByTestId("image-dockerfile-pick-local-app").selectOption({ index: 1 });
  await expect(page.getByTestId("image-context-local-app")).toHaveValue("app");
  await expect(page.getByTestId("image-dockerfile-local-app")).toHaveValue("Dockerfile");
  await expect(page.getByTestId("image-stack-local-app")).toContainText(/Services|Сервисы/);
});

test("service stack checklists bind networks and secrets", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("service-app")).toBeVisible({ timeout: 15_000 });

  await page.getByTestId("map-secrets-add-id").fill("db-pass");
  await page.getByTestId("map-secrets-add").click();
  await page
    .getByTestId("service-secrets-app")
    .locator("label")
    .filter({ hasText: /^db-pass$/ })
    .locator("input")
    .check();
  await expect(
    page.getByTestId("service-secrets-app").locator("label").filter({ hasText: /^db-pass$/ }).locator("input"),
  ).toBeChecked();

  await page
    .getByTestId("service-networks-app")
    .locator("label")
    .filter({ hasText: /^frontend$/ })
    .locator("input")
    .check();
  await expect(
    page
      .getByTestId("service-networks-app")
      .locator("label")
      .filter({ hasText: /^frontend$/ })
      .locator("input"),
  ).toBeChecked();
});

test("resource editor picks source and applies include preset", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-resources")).toBeVisible({ timeout: 15_000 });

  await page.getByTestId("map-resources-add-id").fill("static");
  await page.getByTestId("map-resources-add").click();
  await expect(page.getByTestId("resource-static")).toBeVisible();

  await page.getByTestId("resource-source-pick-static").selectOption("app");
  await expect(page.getByTestId("resource-source-static")).toHaveValue("app");

  await page.getByTestId("resource-static").getByRole("button", { name: "src/**" }).click();
  await expect(page.getByTestId("resource-include-static")).toHaveValue(/\bsrc\/\*\*/);
});
