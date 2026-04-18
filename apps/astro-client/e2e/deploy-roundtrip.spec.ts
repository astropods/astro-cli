import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const AGENT_INGESTION_SCHEDULE = "ingestion-scheduled";
const DEPLOYMENT_INGESTION_SCHEDULE_ID = "dep-ingestion-schedule-1";
const DEPLOY_PAGE = `/deploy/${ACCOUNT}/${AGENT_INGESTION_SCHEDULE}`;

test.beforeEach(async ({ request }) => {
  await request.post("http://127.0.0.1:48787/test/reset");
});

test.describe("deploy then configure round-trip", () => {
  test("variable values survive round-trip to configure page", async ({ page }) => {
    test.setTimeout(90_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill("sk-roundtrip-test-key");

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Daily at midnight" }).click();

    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByLabel("Openai Api Key")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-roundtrip-test-key");
  });

  test("ingestion schedule survives round-trip to configure page", async ({ page }) => {
    test.setTimeout(90_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill("sk-schedule-roundtrip");

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Hourly" }).click();
    await expect(page.getByText("Runs at minute 0")).toBeVisible();

    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByRole("heading", { name: "Ingestion", exact: true })).toBeVisible({ timeout: 20_000 });
    const scheduleSelect = page.getByRole("combobox").filter({ hasText: "Hourly" });
    await expect(scheduleSelect).toBeVisible();
  });

  test("all editable fields survive round-trip together", async ({ page }) => {
    test.setTimeout(90_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill("sk-combined-roundtrip");

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Weekly on Sunday" }).click();

    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByLabel("Openai Api Key")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-combined-roundtrip");

    await expect(page.getByRole("heading", { name: "Ingestion", exact: true })).toBeVisible();
    const scheduleSelect = page.getByRole("combobox").filter({ hasText: "Weekly on Sunday" });
    await expect(scheduleSelect).toBeVisible();
  });
});
