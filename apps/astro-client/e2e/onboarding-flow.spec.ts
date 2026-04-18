import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("full onboarding: create blueprint, initialize, navigate to detail, then deploy", async ({ page }) => {
  test.setTimeout(60_000);

  // ── Step 1: Identity ─────────────────────────────────────────────────────
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled({ timeout: 5_000 });
  await page.getByRole("button", { name: /^continue$/i }).click();

  // ── Step 2: Source — select local ────────────────────────────────────────
  await expect(page.getByText(/starting point/i)).toBeVisible({ timeout: 5_000 });
  await page.getByText(/set up locally/i).click();

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  await createReq;

  // ── Step 3: Publishing panel ─────────────────────────────────────────────
  await expect(page.getByText(/initializing mynewagent/i)).toBeVisible({ timeout: 10_000 });

  // ── Step 4: Auto-nav to blueprint detail ──────────────────────────────────
  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 15_000 });

  // ── Step 5: Blueprint detail — Deploy button is visible (not draft) ───────
  // Two "Deploy this agent" links exist (mobile + desktop); use last() for the visible desktop one.
  await expect(page.getByRole("link", { name: /deploy this agent/i }).last()).toBeVisible({ timeout: 10_000 });
  await page.getByRole("link", { name: /deploy this agent/i }).last().click();

  await page.waitForURL(`**/deploy/${ACCOUNT}/mynewagent`, { timeout: 10_000 });

  // ── Step 6: Deploy form — fill required variable and submit ───────────────
  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 10_000 });
  await page.getByLabel(/openai api key/i).fill("sk-onboarding-test-key");

  const deployReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes("/api/v1/deploy"),
  );
  await page.getByRole("button", { name: /^deploy$/i }).click();
  const req = await deployReq;
  const payload = req.postDataJSON() as { source?: { name?: string }; variables?: Record<string, { value?: string }> };
  expect(payload.source?.name).toBe("mynewagent");
  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-onboarding-test-key");

  await page.waitForURL("**/agents*", { timeout: 20_000 });
});

test("review panel: 'continue setup' button navigates to blueprint detail without waiting for auto-nav poll", async ({ page }) => {
  test.setTimeout(30_000);

  // Delay the blueprint detail poll so auto-nav doesn't fire before we click the button
  await page.route(`**/api/v1/agents/${ACCOUNT}/mynewagent`, async (route) => {
    if (route.request().method() === "GET") {
      await new Promise((resolve) => setTimeout(resolve, 4_000));
    }
    return route.continue();
  });

  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await page.getByRole("button", { name: /^continue$/i }).click();

  await expect(page.getByText(/starting point/i)).toBeVisible({ timeout: 5_000 });
  await page.getByText(/set up locally/i).click();
  await page.getByRole("button", { name: /create blueprint/i }).click();

  await expect(page.getByText(/blueprint registered/i)).toBeVisible({ timeout: 10_000 });

  // While the poll is delayed, the "Continue setup →" button must be visible and functional
  await expect(page.getByRole("button", { name: /continue setup/i })).toBeVisible({ timeout: 5_000 });
  await page.getByRole("button", { name: /continue setup/i }).click();

  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 10_000 });
});

test("blueprint detail deploy button navigates to deploy form", async ({ page }) => {
  test.setTimeout(30_000);

  // Navigate directly to an existing (non-draft) blueprint
  await page.goto(`/${ACCOUNT}/code-reviewer`, { waitUntil: "domcontentloaded" });

  // Wait for blueprint detail to fully load (sidebar deploy button appears)
  await expect(page.getByRole("link", { name: /deploy this agent/i }).last()).toBeVisible({ timeout: 15_000 });

  await page.getByRole("link", { name: /deploy this agent/i }).last().click();
  await page.waitForURL(`**/deploy/${ACCOUNT}/code-reviewer`, { timeout: 10_000 });
});
