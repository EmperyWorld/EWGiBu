import { expect, test } from "@playwright/test";

test("language switch toggles chrome between EN and RU", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("lang-en")).toBeVisible({ timeout: 15_000 });
  await expect(page.locator(".tagline")).toContainText(/Local authoring/i);

  await page.getByTestId("lang-ru").click();
  await expect(page.locator(".tagline")).toContainText(/Локальное редактирование/);
  await expect(page.getByTestId("tab-designer").locator("strong")).toHaveText("Дизайнер");
  await expect(page.getByTestId("save")).toHaveText("Сохранить");

  await page.getByTestId("tab-forms").click();
  await expect(page.getByTestId("map-services")).toContainText("Сервисы");
  await expect(page.getByTestId("service-app")).toContainText("Порты");
  await expect(page.getByTestId("service-add-port-app")).toHaveText("Добавить порт");

  await page.getByTestId("lang-en").click();
  await expect(page.getByTestId("tab-designer").locator("strong")).toHaveText("Designer");
  await expect(page.getByTestId("save")).toHaveText("Save");
  await expect(page.getByTestId("service-add-port-app")).toHaveText("Add port");
});
