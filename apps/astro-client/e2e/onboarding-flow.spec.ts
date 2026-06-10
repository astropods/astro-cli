import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("full onboarding: create blueprint, initialize, navigate to detail, then deploy", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  await page.getByRole("button", { name: /^continue$/i }).click();

  await expect(page.getByText(/starting point/i)).toBeVisible();
  await page.getByText(/set up locally/i).click();

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  await createReq;

  await expect(page.getByText(/initializing mynewagent/i)).toBeVisible();
  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 30_000 });

  await expect(page.getByRole("link", { name: /deploy this agent/i }).last()).toBeVisible();
  await page.getByRole("link", { name: /deploy this agent/i }).last().click();

  await page.waitForURL(`**/deploy/${ACCOUNT}/mynewagent`, { timeout: 20_000 });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible();
  await page.getByLabel("Openai Api Key").fill("sk-onboarding-test-key");

  const deployReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes("/api/v1/deploy"),
  );
  await Promise.all([
    deployReq,
    page.waitForURL("**/agents*", { timeout: 30_000 }),
    page.getByRole("button", { name: /^deploy$/i }).click(),
  ]);

  const req = await deployReq;
  const payload = req.postDataJSON() as { source?: { name?: string }; variables?: Record<string, { value?: string }> };
  expect(payload.source?.name).toBe("mynewagent");
  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-onboarding-test-key");
});

test("review panel: 'continue setup' button navigates to blueprint detail without waiting for auto-nav poll", async ({ page }) => {
  const detailPath = `/api/v1/agents/${ACCOUNT}/mynewagent`;

  // Delay blueprint detail polls so auto-nav does not beat the manual button click.
  await page.route(`**${detailPath}`, async (route) => {
    if (route.request().method() === "GET") {
      await new Promise((resolve) => setTimeout(resolve, 4_000));
    }
    await route.continue();
  });

  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await page.getByRole("button", { name: /^continue$/i }).click();

  await expect(page.getByText(/starting point/i)).toBeVisible();
  await page.getByText(/set up locally/i).click();
  await page.getByRole("button", { name: /create blueprint/i }).click();

  await expect(page.getByText(/blueprint registered/i)).toBeVisible();

  const continueBtn = page.getByRole("button", { name: /continue setup/i });
  await expect(continueBtn).toBeVisible();
  await continueBtn.click();

  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 30_000 });
});

test("blueprint detail deploy button navigates to deploy form", async ({ page }) => {
  await page.goto(`/${ACCOUNT}/code-reviewer`, { waitUntil: "domcontentloaded" });

  const deployLink = page.getByRole("link", { name: /deploy this agent/i }).last();
  await expect(deployLink).toBeVisible();
  await deployLink.click();
  await page.waitForURL(`**/deploy/${ACCOUNT}/code-reviewer`, { timeout: 20_000 });
});
