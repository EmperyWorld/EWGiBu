import { expect, test, type Page } from "@playwright/test";

async function showDeps(page: Page) {
  const drawer = page.getByTestId("dep-drawer");
  await expect(drawer).toBeVisible();
  if ((await drawer.getAttribute("open")) === null) {
    await drawer.locator("summary").click();
  }
}

test("designer maps two-service graph and supports keyboard property focus", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("tab-designer")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("tab-designer").click();
  await expect(page.getByTestId("graph-canvas")).toBeVisible();
  await expect(page.getByTestId("server-graph-status")).toBeVisible();

  await showDeps(page);
  await expect(page.getByRole("table")).toContainText("api");

  await page.getByRole("button", { name: "app" }).first().click();
  await expect(page.getByRole("heading", { name: /Service: app/ })).toBeVisible();
  const img = page.locator("section.panel").filter({ hasText: "Service: app" }).locator("select").first();
  await img.focus();
  await expect(img).toBeFocused();

  await page.getByRole("button", { name: "Auto-layout" }).click();
  await expect(page.getByTestId("status")).toContainText(/Layout|x-ui|updated|dirty|Local/i);
});

test("disconnect removes dependency; connect via selects rejects duplicate", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });

  await showDeps(page);
  await page.getByRole("button", { name: "Disconnect" }).click();
  await expect(page.getByText(/No dependencies/i)).toBeVisible();

  await page.getByTestId("graph-dep-from").selectOption("app");
  await page.getByTestId("graph-dep-to").selectOption("api");
  await page.getByTestId("graph-connect").click();
  await expect(page.getByRole("table")).toContainText("app");
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();

  await page.getByTestId("graph-connect").click();
  await expect(page.getByTestId("status")).toContainText(/duplicate/i);
});

test("canvas can add a service node", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-add-service")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("graph-add-service").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("status")).toContainText(/Local|unsaved|dirty/i);
});

test("canvas connects service to preStart task; phase filter hides services for preStart", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-add-task")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("graph-add-task").click();
  await expect(page.getByRole("heading", { name: /Task: task/ })).toBeVisible({ timeout: 10_000 });

  await showDeps(page);
  const disconnect = page.getByRole("button", { name: "Disconnect" });
  if ((await disconnect.count()) > 0) {
    await disconnect.first().click();
  }

  await page.getByTestId("graph-dep-from").selectOption("app");
  await page.getByTestId("graph-dep-to").selectOption("task");
  await expect(page.getByTestId("graph-dep-condition")).toHaveValue("completedSuccessfully");
  await page.getByTestId("graph-connect").click();
  await expect(page.getByRole("table")).toContainText("task");
  await expect(page.getByRole("table")).toContainText("completedSuccessfully");

  await page.getByTestId("graph-dep-from").selectOption("task");
  const toTexts = await page.getByTestId("graph-dep-to").locator("option").allTextContents();
  expect(toTexts.every((t) => /task/.test(t))).toBeTruthy();
});

test("canvas fit view and delete selected service scrub deps", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-fit-view")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("graph-fit-view").click();
  await expect(page.getByTestId("status")).toContainText(/fitted|Viewport|Local|dirty/i);

  await page.getByTestId("graph-add-service").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("graph-delete-selected")).toBeEnabled();
  await page.getByTestId("graph-delete-selected").click();
  await expect(page.getByTestId("status")).toContainText(/Deleted|Удалено|Local|dirty/i);
  await expect(page.getByRole("heading", { name: /Service: service/ })).toHaveCount(0);
});

test("diamond dependencies and informational network toggle", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });

  await showDeps(page);
  const disconnect = page.getByRole("button", { name: "Disconnect" });
  while ((await disconnect.count()) > 0) {
    await disconnect.first().click();
  }

  await page.getByTestId("graph-add-service").click();
  await page.getByTestId("graph-add-service").click();
  await page.getByTestId("graph-add-service").click();

  await page.getByTestId("graph-dep-from").selectOption("service");
  await page.getByTestId("graph-dep-to").selectOption("api");
  await page.getByTestId("graph-connect").click();

  await page.getByTestId("graph-dep-from").selectOption("service-2");
  await page.getByTestId("graph-dep-to").selectOption("api");
  await page.getByTestId("graph-connect").click();

  await page.getByTestId("graph-dep-from").selectOption("service-3");
  await page.getByTestId("graph-dep-to").selectOption("service");
  await page.getByTestId("graph-connect").click();

  await page.getByTestId("graph-dep-from").selectOption("service-3");
  await page.getByTestId("graph-dep-to").selectOption("service-2");
  await page.getByTestId("graph-connect").click();

  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(4);

  await page.getByTestId("graph-show-info").check();
  await expect(page.getByRole("group", { name: /network frontend/i })).toBeVisible();
});

test("graph shell keyboard shortcuts: A add, Escape clear, Delete remove", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-shell")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("graph-shortcuts")).toBeVisible();

  await page.getByTestId("graph-shell").focus();
  await page.keyboard.press("a");
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible({ timeout: 10_000 });

  await page.getByTestId("graph-shell").focus();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("status")).toContainText(/Selection cleared|Выбор сброшен/i);
  await expect(page.getByRole("heading", { name: /Service: service/ })).toHaveCount(0);

  await page.getByTestId("graph-add-service").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible();
  await page.getByTestId("graph-shell").focus();
  await page.keyboard.press("Delete");
  await expect(page.getByTestId("status")).toContainText(/Deleted|Удалено/i);
  await expect(page.getByRole("heading", { name: /Service: service/ })).toHaveCount(0);
});

test("undo restores deleted service from toolbar", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-add-service")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("undo")).toBeDisabled();

  await page.getByTestId("graph-add-service").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("undo")).toBeEnabled();

  await page.getByTestId("graph-delete-selected").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toHaveCount(0);

  await page.getByTestId("undo").click();
  await expect(page.getByTestId("status")).toContainText(/Undone/i);
  await expect(page.getByLabel(/^service service$/i)).toBeVisible({ timeout: 10_000 });

  await page.getByTestId("redo").click();
  await expect(page.getByTestId("status")).toContainText(/Redone/i);
  await expect(page.getByLabel(/^service service$/i)).toHaveCount(0);
});

test("shift multi-select shows inspector summary and bulk delete", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });

  await page.getByLabel(/^service app$/i).click();
  await expect(page.getByRole("heading", { name: /Service: app/ })).toBeVisible();

  await page.getByLabel(/^service api$/i).click({ modifiers: ["Shift"] });
  await expect(page.getByTestId("prop-multi")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("prop-multi-count")).toContainText(/2/);

  await page.getByTestId("graph-delete-selected").click();
  await expect(page.getByTestId("status")).toContainText(/Deleted|Удалено/i);
  await expect(page.getByLabel(/^service app$/i)).toHaveCount(0);
  await expect(page.getByLabel(/^service api$/i)).toHaveCount(0);

  // One undo restores both (batched replaceProject).
  await page.getByTestId("undo").click();
  await expect(page.getByLabel(/^service app$/i)).toBeVisible();
  await expect(page.getByLabel(/^service api$/i)).toBeVisible();
});

test("info network node opens editable inspector", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-show-info")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("graph-show-info").check();
  await expect(page.getByLabel(/^network frontend$/i)).toBeVisible();

  await page.getByLabel(/^network frontend$/i).click();
  await expect(page.getByRole("heading", { name: /Network: frontend/i })).toBeVisible();
  await expect(page.getByTestId("network-frontend")).toBeVisible();
  await expect(page.getByTestId("network-stack-frontend")).toContainText(/app|api/i);

  await page.getByTestId("network-external-frontend").selectOption("yes");
  await expect(page.getByTestId("network-external-name-frontend")).toBeVisible();
  await page.getByTestId("network-external-name-frontend").fill("edge");
  await expect(page.getByTestId("status")).toContainText(/Local|unsaved|dirty|Manifest/i);
});

test("disconnected service and condition edit via accessible table", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("graph-canvas")).toBeVisible({ timeout: 15_000 });

  await showDeps(page);
  await page.getByRole("button", { name: "Disconnect" }).click();
  await expect(page.getByText(/No dependencies/i)).toBeVisible();

  await page.getByTestId("graph-add-service").click();
  await expect(page.getByRole("heading", { name: /Service: service/ })).toBeVisible();
  // New service stays disconnected from the dependency table.
  await expect(page.getByRole("table")).toHaveCount(0);

  await page.getByTestId("graph-dep-from").selectOption("app");
  await page.getByTestId("graph-dep-to").selectOption("api");
  await page.getByTestId("graph-connect").click();
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(1);
  await expect(page.getByTestId("graph-connect-form")).toBeVisible();

  // Cannot switch to healthy without healthcheck — status announces.
  await page.getByTestId("dep-condition-app-api").selectOption("healthy");
  await expect(page.getByTestId("status")).toContainText(/healthcheck/i);
});
