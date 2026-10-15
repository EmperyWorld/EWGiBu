import { expect, test } from "@playwright/test";
import { openImportAcceptStep } from "./importWizard";

test("runtime guide shows CLI script from workspace API", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("runtime-open").click();
  await expect(page.getByTestId("runtime-guide")).toBeVisible();
  await expect(page.getByTestId("runtime-script")).toContainText("iacpack lock", { timeout: 10_000 });
  await expect(page.getByTestId("runtime-script")).toContainText("iacpack apply");
  await page.getByTestId("runtime-close").click();
  await expect(page.getByTestId("runtime-guide")).toHaveCount(0);
});

test("save-as dialog submits path without window.prompt", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("compose.yaml");
  await page.getByTestId("import-name").fill("save-as-draft");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await openImportAcceptStep(page);
  await page.getByTestId("import-accept").click();

  await page.getByTestId("save-as-open").click();
  await expect(page.getByTestId("save-as-dialog")).toBeVisible();
  await page.getByTestId("save-as-path").fill("iacpack.e2e-copy.yaml");
  await page.getByTestId("save-as-submit").click();
  await expect(page.getByTestId("save-as-dialog")).toHaveCount(0, { timeout: 10_000 });
  await expect(page.getByTestId("runtime-guide")).toBeVisible({ timeout: 10_000 });
});
