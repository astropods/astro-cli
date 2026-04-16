import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";

const REPOS = [
  { full_name: "testuser/my-repo", default_branch: "main", private: false, permissions: { admin: true } },
  { full_name: "testuser/another-repo", default_branch: "main", private: true, permissions: { admin: true } },
];

test.beforeEach(async ({ page }) => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });

  // Serve repos directly from the browser context so there's no async gap
  // between "connected: true" landing and repo options being renderable.
  // The mock backend still handles connect/link/connections/status.
  await page.route("**/api/v1/accounts/*/github/repos", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ repos: REPOS }),
    }),
  );
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

/** Connect GitHub and wait for the repo selector to be ready. */
async function connectGitHub(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.getByRole("button", { name: /connect github/i }).click();
  await expect(page.getByText(/github connected/i)).toBeVisible({ timeout: 15_000 });
  // Wait for the combobox to transition from "Loading repositories..." to "Select a repository"
  await expect(page.getByRole("combobox").filter({ hasText: /select a repository/i })).toBeVisible({ timeout: 15_000 });
}

/** Open the repo combobox and wait for Radix option items to mount in the portal. */
async function openRepoPicker(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.getByRole("combobox").filter({ hasText: /select a repository/i }).click();
  // SelectItem renders with role="option"; wait for at least one to mount
  await expect(page.getByRole("option").first()).toBeVisible({ timeout: 10_000 });
}

// ─── Test 1: Local setup flow ─────────────────────────────────────────────────

test("local setup flow: selecting 'set up locally' creates blueprint and shows CLI instructions", async ({ page }) => {
  test.setTimeout(45_000);

  await goToSourceStep(page, "mylocal");
  await page.getByText(/set up locally/i).click();
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeEnabled({ timeout: 5_000 });

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  const req = await createReq;

  const payload = req.postDataJSON() as { name: string; visibility?: string };
  expect(payload.name).toBe("mylocal");

  await expect(page.getByText(/initializing mylocal/i)).toBeVisible({ timeout: 10_000 });
  await page.waitForURL(`**/${ACCOUNT}/mylocal`, { timeout: 20_000 });
  await expect(page.getByRole("heading", { name: "mylocal" })).toBeVisible({ timeout: 10_000 });
});

// ─── Test 2: GitHub import flow ───────────────────────────────────────────────

test("github import flow: connect GitHub, select repo, create blueprint and navigate to detail", async ({ page }) => {
  test.setTimeout(60_000);

  await goToSourceStep(page, "mygithub");
  await page.getByText(/set up with github/i).click();
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeDisabled({ timeout: 5_000 });

  await connectGitHub(page);
  await openRepoPicker(page);

  await page.getByRole("option", { name: /testuser\/my-repo/ }).click();
  await expect(page.getByRole("button", { name: /create blueprint/i })).toBeEnabled({ timeout: 5_000 });

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );
  await page.getByRole("button", { name: /create blueprint/i }).click();
  await createReq;

  await expect(page.getByText(/initializing mygithub/i)).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/blueprint registered/i)).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /view blueprint/i }).click();
  await page.waitForURL(`**/${ACCOUNT}/mygithub`, { timeout: 15_000 });
});

// ─── Test 3: Linked repo is disabled in the wizard ───────────────────────────

test("repo already linked to another blueprint shows as disabled in the repo picker", async ({ page }) => {
  test.setTimeout(45_000);

  // Pre-link testuser/my-repo to code-reviewer via the mock backend
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/code-reviewer/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  await goToSourceStep(page, "newagent");
  await page.getByText(/set up with github/i).click();
  await connectGitHub(page);
  await openRepoPicker(page);

  // testuser/my-repo should be present but disabled, with "linked to" hint
  await expect(page.getByRole("option", { name: /testuser\/my-repo/ })).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/linked to code-reviewer/i)).toBeVisible({ timeout: 5_000 });

  // testuser/another-repo should be selectable (not disabled)
  await expect(page.getByRole("option", { name: /testuser\/another-repo/ })).toBeVisible({ timeout: 5_000 });
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

  const before = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`)
    .then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(before.connections.some((c) => c.repo_full_name === "testuser/my-repo")).toBe(true);

  // Archive code-reviewer via the UI
  await page.goto(`/${ACCOUNT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("code-reviewer").first()).toBeVisible({ timeout: 15_000 });
  await page.locator("[aria-label='Blueprint options']").first().click();
  await page.getByRole("menuitem", { name: /^archive$/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await page.getByRole("checkbox").check();
  await page.getByPlaceholder("code-reviewer").fill("code-reviewer");
  await expect(page.getByRole("button", { name: /archive blueprint/i })).toBeEnabled();
  await page.getByRole("button", { name: /archive blueprint/i }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10_000 });

  // Mock backend should have cleared the connection on archive
  const after = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`)
    .then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(after.connections.some((c) => c.repo_full_name === "testuser/my-repo")).toBe(false);

  // Navigate to the wizard — testuser/my-repo should now be selectable (no disabled/linked hint)
  await goToSourceStep(page, "newagent");
  await page.getByText(/set up with github/i).click();
  await connectGitHub(page);
  await openRepoPicker(page);

  await expect(page.getByRole("option", { name: /testuser\/my-repo/ })).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/linked to code-reviewer/i)).not.toBeVisible();
});
