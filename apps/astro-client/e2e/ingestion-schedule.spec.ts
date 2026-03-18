import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const AGENT_INGESTION_SCHEDULE = "ingestion-scheduled";
const DEPLOYMENT_INGESTION_SCHEDULE_ID = "dep-ingestion-schedule-1";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const MOCK_BACKEND = "http://localhost:48787";

interface DeployPayload {
  variables?: Record<string, { value?: string }>;
  ingestion?: Record<string, { trigger?: { type?: string; schedule?: string } }>;
}

test.describe("deploy page", () => {
  test("schedule picker is visible and blocks deploy when empty", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_INGESTION_SCHEDULE}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole("heading", { name: "Ingestion", exact: true })).toBeVisible();
    await expect(page.getByText("Select a schedule")).toBeVisible();

    await page.getByLabel("Openai Api Key").fill("sk-test-value");

    await page.getByRole("button", { name: /deploy/i }).click();

    await expect(page.getByText("A schedule is required")).toBeVisible();
  });

  test("deploy with preset schedule includes cron in payload", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_INGESTION_SCHEDULE}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill("sk-test-value");

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Daily at midnight" }).click();

    await expect(page.getByText("Runs at 12:00 AM")).toBeVisible();

    const deployRequest = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        request.url().includes("/api/v1/deploy"),
    );

    await Promise.all([
      deployRequest,
      page.waitForURL("**/agents", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    const payload = (await deployRequest).postDataJSON() as DeployPayload;

    expect(payload.ingestion?.scheduled).toBeDefined();
    expect(payload.ingestion?.scheduled?.trigger?.schedule).toBe("0 0 * * *");

    await page.waitForLoadState("networkidle");
  });

  test("deploy with custom schedule includes assembled cron in payload", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_INGESTION_SCHEDULE}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await page.getByLabel("Openai Api Key").fill("sk-test-value");

    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Custom schedule" }).click();

    const minuteSelect = page.locator('[data-slot="select-trigger"]').filter({ hasText: "Every minute" });
    await minuteSelect.click();
    await page.getByRole("option", { name: "30" }).click();

    const hourSelect = page.locator('[data-slot="select-trigger"]').filter({ hasText: "Every hour" });
    await hourSelect.click();
    await page.getByRole("option", { name: "9 AM" }).click();

    const deployRequest = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        request.url().includes("/api/v1/deploy"),
    );

    await Promise.all([
      deployRequest,
      page.waitForURL("**/agents", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    const payload = (await deployRequest).postDataJSON() as DeployPayload;

    expect(payload.ingestion?.scheduled?.trigger?.schedule).toBe("30 9 * * *");

    await page.waitForLoadState("networkidle");
  });
});

test.describe("configure page", () => {
  test.beforeEach(async () => {
    await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
  });

  test("prefilled schedule is editable on configure page", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByRole("heading", { name: "Ingestion", exact: true })).toBeVisible({ timeout: 20_000 });

    const scheduleSelect = page.getByRole("combobox").filter({ hasText: "Daily at midnight" });
    await expect(scheduleSelect).toBeVisible();

    await scheduleSelect.click();
    await page.getByRole("option", { name: "Hourly" }).click();

    await expect(page.getByText("Runs at minute 0")).toBeVisible();
    await expect(page.getByRole("button", { name: /save.*redeploy/i })).toBeVisible();

    const deployRequest = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        request.url().includes("/api/v1/deploy"),
    );

    await Promise.all([
      deployRequest,
      page.getByRole("button", { name: /save.*redeploy/i }).click(),
    ]);

    const payload = (await deployRequest).postDataJSON() as DeployPayload;

    expect(payload.ingestion?.scheduled?.trigger?.schedule).toBe("0 * * * *");

    await page.waitForLoadState("networkidle");
  });

  test("manual triggers hidden when deployment has no manual_ingestions", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByRole("heading", { name: "Messaging" })).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText("Manual Triggers")).not.toBeVisible();
  });

  test("trigger manual ingestion, navigate to detail, see running pod", async ({ page }) => {
    test.setTimeout(60_000);

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/configure/deployment`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByText("Manual Triggers")).toBeVisible({ timeout: 20_000 });
    const manualBtn = page.getByRole("button", { name: /^Manual$/i });
    const fullSyncBtn = page.getByRole("button", { name: /full sync/i });
    await expect(manualBtn).toBeVisible();
    await expect(fullSyncBtn).toBeVisible();

    const triggerRequest = page.waitForRequest(
      (req) =>
        req.method() === "POST" &&
        req.url().includes("/ingestion/manual/trigger"),
    );

    await manualBtn.click();

    const req = await triggerRequest;
    expect(req.url()).toContain(
      `/deployments/${DEPLOYMENT_INGESTION_SCHEDULE_ID}/ingestion/manual/trigger`,
    );

    await expect(manualBtn).toBeDisabled();

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_INGESTION_SCHEDULE_ID}`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByText("Containers")).toBeVisible({ timeout: 20_000 });
    const ingestionPod = page.getByText(/ingestion.*manual/i);
    await expect(ingestionPod).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Running")).toBeVisible();
  });
});
