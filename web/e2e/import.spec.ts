import { expect, test } from "@playwright/test";
import {
  openImportAcceptStep,
  openImportOnboardStep,
  openImportPathsStep,
  openImportReviewStep,
} from "./importWizard";

test("import browse uploads compose into workspace for propose", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await expect(page.getByTestId("import-compose-drop")).toBeVisible();
  const input = page.getByTestId("import-compose-drop-input");
  await input.setInputFiles({
    name: "dropped-compose.yaml",
    mimeType: "text/yaml",
    buffer: Buffer.from(
      ["services:", "  drop:", "    image: docker.io/library/busybox:1.36", ""].join("\n"),
      "utf8",
    ),
  });
  await expect(page.getByTestId("import-compose")).toHaveValue(/dropped-compose\.yaml/, {
    timeout: 10_000,
  });
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await page.getByTestId("import-cancel").click();
});

test("import beginner wizard omits folders step for minimal compose", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("compose.yaml");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-wizard-tab-paths")).toHaveCount(0);
  await expect(page.getByTestId("import-wizard-next")).toBeFocused();
  await page.getByTestId("import-wizard-next").click();
  await expect(page.getByTestId("import-step-onboard")).toBeVisible();
  await page.getByTestId("import-wizard-next").click();
  await expect(page.getByTestId("import-accept")).toBeVisible();
  await page.getByTestId("import-cancel").click();
});

test("import draft from Compose propose → accept (memory only)", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("import-open")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("import-open").click();
  await expect(page.getByTestId("import-draft")).toBeVisible();

  await page.getByTestId("import-compose").fill("compose.yaml");
  await page.getByTestId("import-name").fill("shop-draft");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-summary-text")).toContainText("2");
  await openImportAcceptStep(page);
  await page.getByTestId("import-accept").click();
  await expect(page.getByTestId("import-next-steps")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-draft")).toHaveCount(0);

  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("service-web")).toBeVisible({ timeout: 10_000 });

  await page.getByRole("button", { name: /Reload|Перезагрузить/i }).click();
  await expect(page.getByTestId("service-app")).toBeVisible({ timeout: 15_000 });
});

test("import privileged Compose shows policy diagnostics and refusals", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("import-open")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("compose.privileged.yaml");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-diagnostics")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-zero-services")).toBeVisible();
  await expect(page.getByTestId("import-refusals")).toBeVisible();
  await expect(page.getByTestId("import-refusal-policy")).toContainText(/devices|privileged/i);
  await page.getByTestId("import-cancel").click();
});

test("discover lists stack-like compose names for import multi-select", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await expect(page.getByTestId("import-compose-list")).toBeVisible();
  await expect(page.getByText("compose.yaml", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("stack9.yml", { exact: true })).toBeVisible();
  await expect(page.getByTestId("import-compose-check-stack9.yml")).toBeVisible();
});

test("multi-compose propose merges and accepts draft", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await expect(page.getByTestId("import-compose-check-compose.yaml")).toBeVisible({ timeout: 10_000 });
  await page.getByTestId("import-compose-check-compose.yaml").check();
  await page.getByTestId("import-compose-check-stack9.yml").check();
  await page.getByTestId("import-name").fill("multi-draft");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-summary-text")).toContainText("3");
  await openImportAcceptStep(page);
  await page.getByTestId("import-accept").click();
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("service-worker")).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: /Reload|Перезагрузить/i }).click();
  await expect(page.getByTestId("service-app")).toBeVisible({ timeout: 15_000 });
});

test("multi-compose collision shows refusal group", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("compose.yaml\nstack-collide.yml");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("import-refusals")).toBeVisible();
  await expect(page.getByTestId("import-refusal-collision")).toBeVisible();
  await page.getByTestId("import-cancel").click();
});

test("import stack4 propose can create workspace stubs and refresh draft", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("stack4.yml");
  await page.getByTestId("import-name").fill("stack4-stubs");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("import-summary-text")).toContainText(/[1-9]/);
  await openImportPathsStep(page);
  await expect(page.getByTestId("import-create-stubs")).toBeVisible();
  await expect(page.getByTestId("import-stub-preview")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("import-prepare-workspace").click();
  await expect(page.getByTestId("import-notice")).toContainText(/import-draft|prepared|готов/i, {
    timeout: 15_000,
  });
  await expect(page.getByTestId("import-stub-report")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("import-cancel").click();
});

test("import onboarding shows parameter gaps and secret stubs without values", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("import-open").click();
  await page.getByTestId("import-compose").fill("compose.secrets.yaml");
  await page.getByTestId("import-propose").click();
  await expect(page.getByTestId("import-summary")).toBeVisible({ timeout: 10_000 });
  await openImportOnboardStep(page);
  await expect(page.getByTestId("import-parameter-gaps")).toBeVisible();
  await expect(page.getByTestId("import-secret-suggestions")).toBeVisible();
  await expect(page.getByTestId("import-secret-db-password")).toBeVisible();
  await expect(page.getByTestId("import-env-example")).toBeVisible();
  await expect(page.getByTestId("import-env-example")).not.toContainText(/SECRET_CANARY|password123|supersecret/i);
  await openImportReviewStep(page);
  await expect(page.getByTestId("import-diagnostics")).not.toContainText(/SECRET_CANARY|password123|supersecret/i);
  await openImportOnboardStep(page);
  await page.getByTestId("import-param-default-public-host").fill("shop.example");
  await openImportAcceptStep(page);
  await page.getByTestId("import-accept").click();
  await expect(page.getByTestId("import-draft")).toHaveCount(0);
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("service-api")).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: /Reload|Перезагрузить/i }).click();
  await expect(page.getByTestId("service-app")).toBeVisible({ timeout: 15_000 });
});
