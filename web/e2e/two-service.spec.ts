import { expect, test } from "@playwright/test";
import { cpSync, readFileSync } from "node:fs";
import { join } from "node:path";

function restoreFixture() {
  const marker = join(process.cwd(), ".e2e-workspace");
  const ws = readFileSync(marker, "utf8").trim();
  const fixture = join(process.cwd(), "..", "testdata", "ui", "two-service", "iacpack.yaml");
  cpSync(fixture, join(ws, "iacpack.yaml"));
}

test("two-service authoring journey without editing YAML", async ({ page }) => {
  restoreFixture();
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "iacpack" })).toBeVisible();
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("meta-name")).toBeVisible({ timeout: 15_000 });

  await expect(page.getByTestId("service-app")).toBeVisible();
  await expect(page.getByTestId("service-api")).toBeVisible();

  await page.getByTestId("meta-name").fill("shop-ui");
  await page.getByTestId("meta-version").fill("0.2.0");
  await page.getByTestId("service-port-app").fill("9090");
  await expect(
    page.getByTestId("service-depends-app").locator("label").filter({ hasText: /^api$/ }).locator("input"),
  ).toBeChecked();

  await page.getByTestId("save").click();
  await expect(page.getByTestId("status")).toContainText("Saved", { timeout: 20_000 });

  await page.getByTestId("validate").click();
  await expect(page.getByTestId("status")).toContainText(/Validation/i, { timeout: 20_000 });

  await page.getByTestId("preview").click();
  await expect(page.getByTestId("compose-preview")).toBeVisible({ timeout: 60_000 });
  await expect(page.getByTestId("compose-preview")).not.toHaveText("(empty)");
  await expect(page.getByTestId("artifact-tree")).toBeVisible();

  await page.getByTestId("compose-preview").focus();
  await expect(page.getByTestId("compose-preview")).toBeFocused();
});
