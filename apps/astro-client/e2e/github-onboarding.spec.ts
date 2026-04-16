import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

/** Navigate to /new/custom, fill name, and click Continue to reach the source step. */
async function goToSourceStep(page: Parameters<Parameters<typeof test>[1]>[0], name: string) {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 15_000 });
  await page.getByPlaceholder("my-agent").fill(name);
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled({ timeout: 10_000 });
  await page.getByRole("button", { name: /^continue$/i }).click();
  await expect(page.getByText(/starting point/i)).toBeVisible({ timeout: 10_000 });
}

/** Connect GitHub and wait for the repo dropdown to be ready (repos loaded). */
async function connectGitHubAndWaitForRepos(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.getByRole("button", { name: /connect github/i }).click();
  // Wait for connection confirmation and for repos to finish loading
  await expect(page.getByText(/github connected/i)).toBeVisible({ timeout: 15_000 });
  // The combobox shows "Select a repository" only after repos have loaded
  await expect(page.getByRole("combobox").filter({ hasText: /select a repository/i })).toBeVisible({ timeout: 15_000 });
}

// ─── Test 1: Local setup flow ─────────────────────────────────────────────────

test("local setup flow: selecting 'set up locally' creates blueprint and shows CLI instructions", async ({ page }) => {
  test.setTimeout(45_000);

  await goToSourceStep(page, "mylocal");

  // Select "Set up locally"
  await page.getByText(/set up locally/i).click();

  // "Create blueprint" should now be enabled
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeEnabled({ timeout: 5_000 });

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  const req = await createReq;

  const payload = req.postDataJSON() as { name: string; visibility?: string };
  expect(payload.name).toBe("mylocal");

  // Publishing state visible
  await expect(page.getByText(/initializing mylocal/i)).toBeVisible({ timeout: 10_000 });

  // Auto-nav to blueprint detail
  await page.waitForURL(`**/${ACCOUNT}/mylocal`, { timeout: 20_000 });

  // Blueprint detail page loads (name visible in breadcrumb / header)
  await expect(page.getByRole("heading", { name: "mylocal" })).toBeVisible({ timeout: 10_000 });
});

// ─── Test 2: GitHub import flow ───────────────────────────────────────────────

test("github import flow: connect GitHub, select repo, create blueprint and navigate to detail", async ({ page }) => {
  test.setTimeout(60_000);

  await goToSourceStep(page, "mygithub");

  // Select "Set up with GitHub"
  await page.getByText(/set up with github/i).click();

  // "Create blueprint" is disabled until repo is selected
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeDisabled({ timeout: 5_000 });

  // Click "Connect GitHub" — mock returns connected: true immediately
  const connectReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/accounts/${ACCOUNT}/github/connect`),
  );
  await page.getByRole("button", { name: /connect github/i }).click();
  await connectReq;

  // Wait for repos to load (combobox transitions from "Loading repositories..." to "Select a repository")
  await expect(page.getByText(/github connected/i)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("combobox").filter({ hasText: /select a repository/i })).toBeVisible({ timeout: 15_000 });

  // Open dropdown and select a repo
  await page.getByRole("combobox").filter({ hasText: /select a repository/i }).click();
  await expect(page.getByText("testuser/my-repo")).toBeVisible({ timeout: 15_000 });
  await page.getByText("testuser/my-repo").click();

  // "Create blueprint" is now enabled
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeEnabled({ timeout: 5_000 });

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  await createReq;

  // Publishing state visible
  await expect(page.getByText(/initializing mygithub/i)).toBeVisible({ timeout: 10_000 });

  // Review step — for GitHub import path, user clicks "View blueprint →" to navigate
  await expect(page.getByText(/blueprint registered/i)).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /view blueprint/i }).click();

  await page.waitForURL(`**/${ACCOUNT}/mygithub`, { timeout: 15_000 });
});

// ─── Test 3: Linked repo is disabled in the wizard ───────────────────────────

test("repo already linked to another blueprint shows as disabled in the repo picker", async ({ page }) => {
  test.setTimeout(45_000);

  // Pre-link testuser/my-repo to the existing "code-reviewer" blueprint
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/code-reviewer/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  await goToSourceStep(page, "newagent");

  // Select GitHub path, connect, and wait for repos to load
  await page.getByText(/set up with github/i).click();
  await connectGitHubAndWaitForRepos(page);

  // Open the repo dropdown
  await page.getByRole("combobox").filter({ hasText: /select a repository/i }).click();

  // testuser/my-repo should show as disabled with "linked to code-reviewer"
  await expect(page.getByText("testuser/my-repo")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(/linked to code-reviewer/i)).toBeVisible({ timeout: 5_000 });

  // testuser/another-repo should be visible and not disabled
  await expect(page.getByText("testuser/another-repo")).toBeVisible({ timeout: 5_000 });
});

// ─── Test 4: Archiving a blueprint releases its repo ─────────────────────────

test("archiving a blueprint releases its GitHub repo so it can be reused", async ({ page }) => {
  test.setTimeout(60_000);

  // Link testuser/my-repo to code-reviewer
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/code-reviewer/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  // Verify the link exists
  const before = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(before.connections.some((c) => c.repo_full_name === "testuser/my-repo")).toBe(true);

  // Archive code-reviewer via the UI
  await page.goto(`/${ACCOUNT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("code-reviewer").first()).toBeVisible({ timeout: 15_000 });

  const blueprintCard = page.locator("[aria-label='Blueprint options']").first();
  await blueprintCard.click();
  await page.getByRole("menuitem", { name: /^archive$/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await page.getByRole("checkbox").check();
  await page.getByPlaceholder("code-reviewer").fill("code-reviewer");
  await expect(page.getByRole("button", { name: /archive blueprint/i })).toBeEnabled();
  await page.getByRole("button", { name: /archive blueprint/i }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10_000 });

  // Verify the mock connection is now gone
  const after = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(after.connections.some((c) => c.repo_full_name === "testuser/my-repo")).toBe(false);

  // Navigate to the wizard — testuser/my-repo should no longer be disabled
  await goToSourceStep(page, "newagent");
  await page.getByText(/set up with github/i).click();
  await connectGitHubAndWaitForRepos(page);

  await page.getByRole("combobox").filter({ hasText: /select a repository/i }).click();
  await expect(page.getByText("testuser/my-repo")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(/linked to code-reviewer/i)).not.toBeVisible();
});
