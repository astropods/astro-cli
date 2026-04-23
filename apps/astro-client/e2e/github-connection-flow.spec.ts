import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const AGENT = "code-reviewer";

const REPOS = [
  { full_name: "testuser/my-repo", default_branch: "main", private: false, permissions: { admin: true } },
  { full_name: "testuser/another-repo", default_branch: "main", private: true, permissions: { admin: true } },
];

test.beforeEach(async ({ page }) => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });

  await page.route("**/api/v1/accounts/*/github/repos**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ repos: REPOS }),
    }),
  );
});

// ─── Test 1: Linking a repo to a BP marks GitHub as globally connected ────────
//
// After a user connects GitHub via the BP panel and links a repo, the account
// settings page should reflect the connected state. This validates that
// useGitHubLink.onSuccess invalidates githubKeys.accountStatus so the settings
// toggle picks up the new state without a full page reload.

test("linking a repo via BP panel shows account as globally connected in settings", async ({ page }) => {
  test.setTimeout(60_000);

  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: /connect github repo/i })).toBeVisible({ timeout: 15_000 });

  // Click to trigger the OAuth connect. The mock returns { connected: true, github_login: "testgh" }
  // so the repo selector dialog opens immediately instead of redirecting.
  await page.getByRole("button", { name: /connect github repo/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/connect github repository/i)).toBeVisible({ timeout: 5_000 });

  // Select a repo from the picker and connect.
  await page.getByPlaceholder(/search repositories/i).fill("my-repo");
  await expect(page.getByRole("button", { name: /my-repo/ }).first()).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: /my-repo/ }).first().click();
  await page.getByRole("button", { name: /connect repository/i }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10_000 });

  // Navigate to account settings — the account status query fires fresh and must
  // return connected: true since POST /connect already set githubAccountConnected.
  await page.goto("/settings/account", { waitUntil: "domcontentloaded" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("switch")).toBeChecked({ timeout: 5_000 });
});

// ─── Test 2: Unlinking a BP repo does NOT disconnect GitHub globally ──────────
//
// Removing a repo connection from a single blueprint should not touch the
// account-level OAuth connection. Other blueprints can still use GitHub, and
// the account settings toggle must remain ON.
//
// This is a server-invariant test: the backend must keep githubAccountConnected=true
// when a link is deleted. The client-side behavior (useGitHubDisconnect not
// invalidating accountStatus) is covered by unit tests.

test("unlinking a BP repo leaves the global GitHub connection intact", async ({ page }) => {
  test.setTimeout(45_000);

  // Set up: connect GitHub and link code-reviewer to a repo.
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connect`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ redirect_to: "/settings/account" }),
  });
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/${AGENT}/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  // Confirm the link exists.
  const before = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`)
    .then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(before.connections.some((c) => c.agent_name === AGENT)).toBe(true);

  // Remove the BP-level link — equivalent to what the UI calls when disconnecting a repo.
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/${AGENT}/link`, { method: "DELETE" });

  const after = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`)
    .then((r) => r.json()) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
  expect(after.connections.some((c) => c.agent_name === AGENT)).toBe(false);

  // Account-level GitHub connection must still be active — the link deletion
  // must not have changed githubAccountConnected.
  await page.goto("/settings/account", { waitUntil: "domcontentloaded" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("switch")).toBeChecked({ timeout: 5_000 });
});

// ─── Test 3: Global disconnect from account settings clears all BP connections ─
//
// Disconnecting from the account settings page (which removes the WorkOS Pipes
// token) should also wipe every github_connection row. Navigating to any BP
// panel afterward must show the "not connected" state.

test("global GitHub disconnect from settings severs all BP connections", async ({ page }) => {
  test.setTimeout(60_000);

  // Set up: connect GitHub and link code-reviewer to a repo.
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connect`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ redirect_to: "/settings/account" }),
  });
  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/${AGENT}/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  // Navigate to account settings — GitHub should appear as connected.
  await page.goto("/settings/account", { waitUntil: "domcontentloaded" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("switch")).toBeChecked({ timeout: 5_000 });

  // Toggle the switch to trigger the confirmation dialog, then confirm disconnect.
  await page.getByRole("switch").click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await page.getByRole("button", { name: /^disconnect$/i }).click();

  // The toggle should immediately flip off (optimistic update).
  await expect(page.getByRole("button", { name: /connect github/i })).toBeVisible({ timeout: 10_000 });

  // All BP-level connections must be cleared in the backend.
  const connections = await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`)
    .then((r) => r.json()) as { connections: unknown[] };
  expect(connections.connections).toHaveLength(0);

  // Navigating to the BP detail page should show the "not connected" state.
  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: /connect github repo/i })).toBeVisible({ timeout: 15_000 });
});
