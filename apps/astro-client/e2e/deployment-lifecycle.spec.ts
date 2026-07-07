import { expect, test, type Locator, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await resetMockBackend();
});

function statusSwitch(toggle: Locator) {
  return toggle.getByRole("switch");
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

async function pauseDeployment(page: Page, toggle: Locator) {
  const stopReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/stop`),
  );
  await expect(statusSwitch(toggle)).toBeEnabled({ timeout: 5_000 });
  await statusSwitch(toggle).click();
  await stopReq;

  // After query invalidation the mock returns status: "stopped" → label changes to Paused.
  // Scope to the toggle: pod tiles also render "Paused" once the deployment is paused,
  // so an unscoped getByText would fail strict-mode with two matches.
  await expect(toggle.getByText("Paused")).toBeVisible({ timeout: 15_000 });
}

async function simulateRuntimeEmptyBeforePausedReads(page: Page) {
  const pausedReads = deferred();
  const runtimeRead = deferred();
  const pausedDetailRead = deferred();
  const pausedStatusRead = deferred();
  let stopAcked = false;

  await page.route(`**/api/v1/deployments/${DEPLOYMENT_ID}/stop`, async (route) => {
    const response = await route.fetch();
    stopAcked = true;
    await route.fulfill({ response });
  });

  await page.route(`**/api/v1/deployments/${DEPLOYMENT_ID}`, async (route) => {
    if (route.request().method() !== "GET" || !stopAcked) {
      await route.fallback();
      return;
    }
    await pausedReads.promise;
    const response = await route.fetch();
    const body = await response.json() as { deployment?: Record<string, unknown> };
    pausedDetailRead.resolve();
    await route.fulfill({
      status: response.status(),
      contentType: "application/json",
      body: JSON.stringify({
        ...body,
        deployment: {
          ...(body.deployment ?? {}),
          status: "stopped",
        },
      }),
    });
  });

  await page.route(`**/api/v1/deployments/${DEPLOYMENT_ID}/status`, async (route) => {
    if (!stopAcked) {
      await route.fallback();
      return;
    }
    await pausedReads.promise;
    pausedStatusRead.resolve();
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        value: "inactive",
        reason: "paused",
        details: "Deployment is paused",
      }),
    });
  });

  await page.route(`**/api/v1/deployments/${DEPLOYMENT_ID}/runtime`, async (route) => {
    if (!stopAcked) {
      await route.fallback();
      return;
    }
    runtimeRead.resolve();
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        runtime: {
          ready: 0,
          replicas: 1,
          messaging_reachable: false,
          workloads: [
            {
              name: "slack-config-full-agent",
              age: "2d",
              containers: [],
            },
          ],
        },
      }),
    });
  });

  return { pausedReads, runtimeRead, pausedDetailRead, pausedStatusRead };
}

async function resumeDeployment(page: Page, toggle: Locator) {
  const wakeupReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/deployments/${DEPLOYMENT_ID}/wakeup`),
  );
  await expect(statusSwitch(toggle)).toBeEnabled({ timeout: 5_000 });
  await statusSwitch(toggle).click();
  await wakeupReq;

  // Mock returns healthy status → label goes back to Active (may pass through Deploying).
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 15_000 });
}

test("status toggle sends stop request and updates to Paused", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  const toggle = page.getByTestId("agent-status-toggle");
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 15_000 });
  await pauseDeployment(page, toggle);
});

test("pause does not flash pod status back to Starting while runtime refetches first", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  const toggle = page.getByTestId("agent-status-toggle");
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("agent", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
  await page.getByText("agent", { exact: true }).first().click();
  await expect(page.getByRole("heading", { name: "agent" })).toBeVisible({ timeout: 5_000 });

  const { pausedReads, runtimeRead, pausedDetailRead, pausedStatusRead } =
    await simulateRuntimeEmptyBeforePausedReads(page);

  try {
    await expect(statusSwitch(toggle)).toBeEnabled({ timeout: 5_000 });
    await statusSwitch(toggle).click();
    await runtimeRead.promise;

    await expect(page.getByText("Paused", { exact: true })).toHaveCount(3, { timeout: 5_000 });
    await expect(page.getByText("Starting", { exact: true })).toHaveCount(0);

    pausedReads.resolve();
    await pausedDetailRead.promise;
    await pausedStatusRead.promise;

    await expect(page.getByText("Paused", { exact: true })).toHaveCount(3, { timeout: 15_000 });
    await expect(page.getByText("Starting", { exact: true })).toHaveCount(0);
  } finally {
    pausedReads.resolve();
  }
  await expect(toggle.getByText("Paused")).toBeVisible({ timeout: 15_000 });
});

test("status toggle sends wakeup request and updates to Active", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });
  const toggle = page.getByTestId("agent-status-toggle");
  await expect(toggle.getByText("Active")).toBeVisible({ timeout: 15_000 });

  await pauseDeployment(page, toggle);
  await resumeDeployment(page, toggle);
});

test("restart via kebab menu sends restart request", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });
  // Wait for the agent identity to fully render (display name visible)
  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 15_000 });

  // Open the agent actions kebab — wait for button to be stable before clicking
  const agentActionsBtn = page.getByRole("button", { name: "Agent actions" });
  await expect(agentActionsBtn).toBeVisible({ timeout: 5_000 });
  await agentActionsBtn.click();

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
