import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("status toggle sends stop request and updates to Paused", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Active")).toBeVisible({ timeout: 15_000 });

  const stopReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/stop`),
  );
  await page.getByRole("switch").click();
  await stopReq;

  // After query invalidation the mock returns status: "stopped" → label changes to Paused
  await expect(page.getByText("Paused")).toBeVisible({ timeout: 10_000 });
});

test("status toggle sends wakeup request and updates to Active", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });
  const toggle = page.getByTestId("agent-status-toggle");
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 15_000 });

  // First pause the deployment
  await page.getByRole("switch").click();
  await expect(toggle.getByText("Paused")).toBeVisible({ timeout: 10_000 });

  // Now resume
  const wakeupReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/wakeup`),
  );
  await page.getByRole("switch").click();
  await wakeupReq;

  // Mock returns healthy status → label goes back to Active
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 10_000 });
});

test("restart via kebab menu sends restart request", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });
  // Wait for the agent identity to fully render (display name visible)
  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 15_000 });

  // Open the agent identity dropdown — wait for button to be stable before clicking
  const agentMenuBtn = page.getByRole("button", { name: "Agent menu" });
  await expect(agentMenuBtn).toBeVisible({ timeout: 5_000 });
  await agentMenuBtn.click();

  // Dropdown opens — wait for it then click "Restart deployment"
  await expect(page.getByRole("menu")).toBeVisible({ timeout: 10_000 });
  await page.getByRole("menuitem", { name: "Restart deployment" }).click();

  // Confirmation dialog appears
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/brief interruption/i)).toBeVisible({ timeout: 5_000 });

  const restartReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/restart`),
  );
  await page.getByRole("button", { name: "Restart" }).click();
  await restartReq;
});

test("configure tab loads prefilled values and save & redeploys", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/configure`, { waitUntil: "domcontentloaded" });

  // The prefilled template for dep-slack-full-1 has OPENAI_API_KEY: sk-existing-value
  // The field renders as a secret input; wait for it to appear
  await expect(page.getByLabel(/openai api key/i)).toBeVisible({ timeout: 15_000 });

  // Update the API key
  await page.getByLabel(/openai api key/i).fill("sk-updated-via-configure");

  const deployReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes("/api/v1/deploy"),
  );
  await page.getByRole("button", { name: /^redeploy$/i }).click();
  const req = await deployReq;

  const body = req.postDataJSON() as { variables?: Record<string, { value?: string }> };
  expect(body.variables?.OPENAI_API_KEY?.value).toBe("sk-updated-via-configure");
});
