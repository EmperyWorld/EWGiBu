import { expect, test } from "@playwright/test";
import { cpSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

function workspaceRoot(): string {
  return readFileSync(join(process.cwd(), ".e2e-workspace"), "utf8").trim();
}

function manifestPath(): string {
  return join(workspaceRoot(), "iacpack.yaml");
}

function restoreFixture() {
  const fixture = join(process.cwd(), "..", "testdata", "ui", "two-service", "iacpack.yaml");
  cpSync(fixture, manifestPath());
}

test.afterEach(() => {
  restoreFixture();
});

test("external clean edit offers reload without page refresh", async ({ page }) => {
  restoreFixture();
  await page.goto("/");
  await expect(page.getByTestId("status")).toContainText(/Loaded|Saved|Validation|Preview/i, {
    timeout: 20_000,
  });
  await expect(page.getByTestId("sync-banner")).toHaveCount(0);

  const path = manifestPath();
  const cur = readFileSync(path, "utf8");
  writeFileSync(path, cur.replace(/version:\s*[^\n]+/, "version: 9.9.9"), "utf8");

  await expect(page.getByTestId("sync-banner")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("sync-banner")).toHaveAttribute("data-sync", "external");
  await page.getByTestId("sync-reload").click();
  await expect(page.getByTestId("sync-banner")).toHaveCount(0, { timeout: 15_000 });
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("meta-version")).toHaveValue("9.9.9");
});

test("dirty local + external edit enters conflict; discard recovers", async ({ page }) => {
  restoreFixture();
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("meta-name")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("meta-name").fill("local-dirty");
  await expect(page.getByTestId("status")).toContainText(/unsaved/i);
  await page.getByTestId("meta-name").focus();

  const path = manifestPath();
  const cur = readFileSync(path, "utf8");
  writeFileSync(path, `${cur}\n# external-marker\n`, "utf8");

  await expect(page.getByTestId("sync-banner")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("sync-banner")).toHaveAttribute("data-sync", "conflict");
  await expect(page.getByTestId("save")).toBeDisabled();
  // Focus moves into the recovery action group (checklist §2).
  await expect(page.getByTestId("sync-discard")).toBeFocused({ timeout: 5_000 });

  await page.getByTestId("sync-discard").click();
  await expect(page.getByTestId("sync-banner")).toHaveCount(0, { timeout: 15_000 });
  await expect(page.getByTestId("meta-name")).not.toHaveValue("local-dirty");
  // Prior field focus is restored after recovery.
  await expect(page.getByTestId("meta-name")).toBeFocused({ timeout: 5_000 });
});

test("invalid external YAML keeps last valid model and blocks save", async ({ page }) => {
  restoreFixture();
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("meta-name")).toBeVisible({ timeout: 15_000 });
  const name = await page.getByTestId("meta-name").inputValue();

  writeFileSync(manifestPath(), "not: valid-project\n", "utf8");
  await expect(page.getByTestId("sync-banner")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("sync-banner")).toHaveAttribute("data-sync", "invalidDisk");
  await expect(page.getByTestId("save")).toBeDisabled();
  await expect(page.getByTestId("meta-name")).toHaveValue(name);
  await expect(page.getByTestId("status")).toContainText(/invalid/i);
});
