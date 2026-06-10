import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT_INGESTION_SCHEDULE = "ingestion-scheduled";
const DEPLOYMENT_INGESTION_SCHEDULE_ID = "dep-ingestion-schedule-1";
const DEPLOY_PAGE = `/deploy/${ACCOUNT}/${AGENT_INGESTION_SCHEDULE}`;
const ROUNDTRIP_SECRET = "sk-roundtrip-test-key";

test.beforeEach(async ({ request }) => {
  await resetMockBackend(request);
});

test.describe("deploy then configure round-trip", () => {
  test("inline secret shows configured state without exposing value", async ({ page }) => {
    test.setTimeout(90_000);

    await page.route("**/deployment-template", async (route) => {
      if (route.request().method() === "POST") {
        const req = route.request().postDataJSON() as { deployment_id?: string } | null;
        // Only configure/redeploy prefill must not echo stored inline secrets.
        if (req?.deployment_id) {
          const body = await route.fetch().then((r) => r.text());
          expect(body).not.toContain(ROUNDTRIP_SECRET);
        }
      }
      await route.continue();
    });

    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill(ROUNDTRIP_SECRET);

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Daily at midnight" }).click();

    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure`,
      { waitUntil: "domcontentloaded" },
    );

    const secretField = page.getByRole("button", { name: /Openai Api Key.*Auto-filled/i });
    await expect(secretField).toBeVisible({ timeout: 20_000 });
    await expect(secretField).toContainText("•••••••");
    await expect(secretField).toContainText("Auto-filled");
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
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure`,
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
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure`,
      { waitUntil: "domcontentloaded" },
    );

    const secretField = page.getByRole("button", { name: /Openai Api Key.*Auto-filled/i });
    await expect(secretField).toBeVisible({ timeout: 20_000 });
    await expect(secretField).toContainText("•••••••");
    await expect(secretField).toContainText("Auto-filled");

    await expect(page.getByRole("heading", { name: "Ingestion", exact: true })).toBeVisible();
    const scheduleSelect = page.getByRole("combobox").filter({ hasText: "Weekly on Sunday" });
    await expect(scheduleSelect).toBeVisible();
  });
});
