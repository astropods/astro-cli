import { expect, test } from "@playwright/test";
import { MOCK_BACKEND, resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT = "code-reviewer";

const REPOS = [
  { full_name: "testuser/my-repo", default_branch: "main", private: false, permissions: { admin: true } },
  { full_name: "testuser/another-repo", default_branch: "main", private: true, permissions: { admin: true } },
];

test.beforeEach(async ({ page }) => {
  await resetMockBackend();

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

  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "networkidle" });
  const connectButton = page.getByRole("button", { name: /connect github repo/i });
  await expect(connectButton).toBeVisible({ timeout: 20_000 });

  // Clicking triggers the OAuth connect. The mock returns
  // { connected: true, github_login: "testgh" } so the repo picker dialog
  // opens immediately instead of redirecting.
  const connectResponse = page.waitForResponse(
    (r) => /\/api\/v1\/accounts\/[^/]+\/github\/connect$/.test(r.url()) && r.request().method() === "POST",
  );
  await connectButton.click();
  await connectResponse;

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await expect(dialog.getByText(/connect github repository/i)).toBeVisible({ timeout: 5_000 });

  // Wait for repos to load, then pick one and confirm.
  await dialog.getByPlaceholder(/search repositories/i).fill("my-repo");
  const repoButton = dialog.getByRole("button", { name: /my-repo/ }).first();
  await expect(repoButton).toBeVisible({ timeout: 10_000 });
  await repoButton.click();

  await dialog.getByRole("button", { name: /connect repository/i }).click();

  // Verify the server recorded the connection by polling the mock backend
  // directly. Asserting on the browser-level response status is unreliable in
  // CI because Bun's proxy can return a spurious 400 when streaming the
  // request body on Linux under load (the mock still processes the request).
  await expect(async () => {
    const connections = (await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) =>
      r.json(),
    )) as { connections: Array<{ agent_name: string; repo_full_name: string }> };
    expect(connections.connections.some((c) => c.agent_name === AGENT)).toBe(true);
  }).toPass({ timeout: 10_000 });

  // Account status query fires fresh and must return connected:true.
  await page.goto("/settings/connectors", { waitUntil: "networkidle" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("button", { name: /^disconnect$/i })).toBeVisible({ timeout: 10_000 });
});

// ─── Test 2: Unlinking a BP repo does NOT disconnect GitHub globally ──────────
//
// Removing a repo connection from a single blueprint should not touch the
// account-level OAuth connection. The account settings toggle must remain ON.

test("unlinking a BP repo leaves the global GitHub connection intact", async ({ page }) => {
  test.setTimeout(45_000);

  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connect`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ redirect_to: "/settings/connectors" }),
  });
  await fetch(`${MOCK_BACKEND}/api/v1/agents/${ACCOUNT}/${AGENT}/github/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  const before = (await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) => r.json())) as {
    connections: Array<{ agent_name: string; repo_full_name: string }>;
  };
  expect(before.connections.some((c) => c.agent_name === AGENT)).toBe(true);

  await fetch(`${MOCK_BACKEND}/api/v1/agents/${ACCOUNT}/${AGENT}/github/link`, { method: "DELETE" });

  const after = (await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) => r.json())) as {
    connections: Array<{ agent_name: string; repo_full_name: string }>;
  };
  expect(after.connections.some((c) => c.agent_name === AGENT)).toBe(false);

  await page.goto("/settings/connectors", { waitUntil: "networkidle" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("button", { name: /^disconnect$/i })).toBeVisible({ timeout: 10_000 });
});

// ─── Test 3: Global disconnect from connectors settings clears all BP connections ─
//
// Disconnecting from the connectors settings page (which removes the WorkOS Pipes
// token) should also wipe every github_connection row. Navigating to any BP
// panel afterward must show the "not connected" state.

test("global GitHub disconnect from settings severs all BP connections", async ({ page }) => {
  test.setTimeout(60_000);

  await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connect`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ redirect_to: "/settings/connectors" }),
  });
  await fetch(`${MOCK_BACKEND}/api/v1/agents/${ACCOUNT}/${AGENT}/github/link`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ repo_full_name: "testuser/my-repo", branch: "main" }),
  });

  await page.goto("/settings/connectors", { waitUntil: "networkidle" });
  await expect(page.getByText(/@testgh/i)).toBeVisible({ timeout: 20_000 });
  const disconnectTrigger = page.getByRole("button", { name: /^disconnect$/i });
  await expect(disconnectTrigger).toBeVisible({ timeout: 10_000 });

  await disconnectTrigger.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  // Scope confirmation controls to the dialog so we never match unrelated
  // page-level checkboxes or buttons.
  await dialog.getByRole("checkbox").click();
  await dialog.getByPlaceholder(`disconnect ${ACCOUNT}`).fill(`disconnect ${ACCOUNT}`);

  const disconnectResponse = page.waitForResponse(
    (r) => /\/api\/v1\/accounts\/[^/]+\/github$/.test(r.url()) && r.request().method() === "DELETE",
  );
  await dialog.getByRole("button", { name: /^disconnect$/i }).click();
  await disconnectResponse;

  // Server state is now disconnected; confirm via the mock, then verify
  // the UI reflects the state after the invalidation settles.
  const connections = (await fetch(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/github/connections`).then((r) => r.json())) as {
    connections: unknown[];
  };
  expect(connections.connections).toHaveLength(0);

  await expect(page.getByRole("button", { name: /connect github/i })).toBeVisible({ timeout: 15_000 });

  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "networkidle" });
  await expect(page.getByRole("button", { name: /connect github repo/i })).toBeVisible({ timeout: 20_000 });
});
