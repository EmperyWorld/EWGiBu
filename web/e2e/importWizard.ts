import type { Page } from "@playwright/test";

/** Review step shows summary, diagnostics, and refusals. */
export async function openImportReviewStep(page: Page): Promise<void> {
  await page.getByTestId("import-wizard-tab-review").click();
}

/** Open the Accept step where the Accept button lives (beginner wizard). */
export async function openImportAcceptStep(page: Page): Promise<void> {
  await page.getByTestId("import-wizard-tab-accept").click();
}

export async function openImportPathsStep(page: Page): Promise<void> {
  await page.getByTestId("import-wizard-tab-paths").click();
}

export async function openImportOnboardStep(page: Page): Promise<void> {
  await page.getByTestId("import-wizard-tab-onboard").click();
}
