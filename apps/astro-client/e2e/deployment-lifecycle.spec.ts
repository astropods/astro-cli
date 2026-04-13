import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("pause button sends stop request and UI updates to Resume", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(AGENT_DETAIL, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 15_000 });

  const stopReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/stop`),
  );
  await page.getByRole("button", { name: "Pause" }).click();
  await stopReq;

  // After query invalidation the mock returns status: "stopped" → button changes to Resume
  await expect(page.getByRole("button", { name: "Resume" })).toBeVisible({ timeout: 10_000 });
});

test("resume button sends wakeup request and UI updates to Pause", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(AGENT_DETAIL, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 15_000 });

  // First pause the deployment
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByRole("button", { name: "Resume" })).toBeVisible({ timeout: 10_000 });

  // Now resume
  const wakeupReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/wakeup`),
  );
  await page.getByRole("button", { name: "Resume" }).click();
  await wakeupReq;

  // Mock returns healthy status → button goes back to Pause
  await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 10_000 });
});

test("restart via kebab menu sends restart request", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(AGENT_DETAIL, { waitUntil: "domcontentloaded" });
  // Wait for the header to fully render (display name visible)
  await expect(page.getByRole("heading", { name: "Slack Full Bot" })).toBeVisible({ timeout: 15_000 });

  // The KebabMenu button (EllipsisHorizontal) lives inside the div alongside the h1 heading
  const kebabButton = page.locator("h1").filter({ hasText: "Slack Full Bot" }).locator("..").locator("button");
  await expect(kebabButton).toBeVisible({ timeout: 5_000 });
  await kebabButton.click();

  // Dropdown opens — click "Restart deployment"
  await page.getByText("Restart deployment").click();

  // Confirmation dialog appears
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/brief interruption/i)).toBeVisible({ timeout: 5_000 });

  const restartReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/restart`),
  );
  await page.getByRole("button", { name: "Restart" }).click();
  await restartReq;
});

test("configure side panel opens, loads prefilled values, and save & redeploys", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(AGENT_DETAIL, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: "Configure" })).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: "Configure" }).click();

  // Panel opens — the prefilledTemplate for dep-slack-full-1 has OPENAI_API_KEY: sk-existing-value
  // The field renders as a secret input; wait for it to appear
  await expect(page.getByLabel(/openai api key/i)).toBeVisible({ timeout: 10_000 });

  // Update the API key
  await page.getByLabel(/openai api key/i).fill("sk-updated-via-panel");

  const deployReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes("/api/v1/deploy"),
  );
  await page.getByRole("button", { name: /^redeploy$/i }).click();
  const req = await deployReq;

  const body = req.postDataJSON() as { variables?: Record<string, { value?: string }> };
  expect(body.variables?.OPENAI_API_KEY?.value).toBe("sk-updated-via-panel");
});
