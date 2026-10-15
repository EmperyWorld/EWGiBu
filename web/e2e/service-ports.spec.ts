import { expect, test } from "@playwright/test";

test("service editor supports multiple ports and mounts", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-services")).toBeVisible({ timeout: 15_000 });

  const forms = page.getByTestId("map-services");
  await expect(forms.getByTestId("service-app")).toBeVisible();
  await forms.getByTestId("service-add-port-app").click();
  await expect(forms.getByTestId("service-port-row-app-1")).toBeVisible();
  await forms.getByTestId("service-port-row-app-1").locator('input[type="number"]').first().fill("9091");

  await page.getByTestId("map-volumes-add-id").fill("data");
  await page.getByTestId("map-volumes-add").click();
  await expect(page.getByTestId("map-volumes")).toContainText("data");

  await forms.getByTestId("service-app-add-mount").click();
  await expect(forms.getByTestId("service-app-mount-row-0")).toBeVisible();
  await expect(forms.getByTestId("service-app-mount-row-0").locator("input").first()).toHaveValue("/data");

  await page.getByTestId("tab-forms").focus();
  await page.keyboard.press("ArrowLeft");
  await expect(page.getByTestId("tab-designer")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("graph-canvas")).toBeVisible();
  await page.getByRole("button", { name: "app" }).first().click();

  const inspector = page.getByRole("region", { name: /Service: app/ });
  await expect(inspector).toBeVisible();
  await expect(inspector.getByTestId("service-port-row-app-1")).toBeVisible();
  await expect(inspector.getByTestId("service-app-mount-row-0")).toBeVisible();
});
