import { expect, test } from "@playwright/test";

test("service env rows and memory MiB editor", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-services")).toBeVisible({ timeout: 15_000 });

  const forms = page.getByTestId("map-services");
  const env = forms.getByTestId("service-env-app");
  await env.getByTestId("service-env-app-add").click();
  await expect(env.getByTestId("service-env-app-row-0")).toBeVisible();
  await env.getByTestId("service-env-app-row-0").locator("input").first().fill("LOG_LEVEL");
  await env.getByTestId("service-env-app-row-0").locator("input").nth(1).fill("info");

  await forms.getByTestId("service-memory-app").fill("256");
  await expect(forms.getByTestId("service-memory-app")).toHaveValue("256");
  await expect(page.getByTestId("status")).toContainText(/dirty|unsaved|Local/i);
});
