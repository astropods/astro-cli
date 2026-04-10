import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";

async function setRole(role: string) {
  await fetch(`${MOCK_BACKEND}/test/set-role`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ role }),
  });
}

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

// --- Admin flow tests ---

test.describe("admin permissions", () => {
  test.beforeEach(async () => {
    await setRole("admin");
  });

  test("admin can see and edit org general settings", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });

    // Wait for the full page to render by checking for danger zone (last section)
    await expect(page.getByText("Danger Zone")).toBeVisible({ timeout: 15_000 });

    // Display name input should be enabled
    const displayNameInput = page.locator("input").first();
    await expect(displayNameInput).toHaveValue("Test Org");
    await expect(displayNameInput).toBeEnabled();

    // Delete button should be enabled
    const deleteBtn = page.getByRole("button", { name: "Delete" });
    await expect(deleteBtn).toBeEnabled();
  });

  test("admin sees invite button on members page", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });

    const inviteBtn = page.getByRole("button", { name: /invite members/i });
    await expect(inviteBtn).toBeVisible({ timeout: 15_000 });
  });

  test("admin sees Secrets & Variables nav", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });

    // Wait for the page to fully render
    await expect(page.getByText("Danger Zone")).toBeVisible({ timeout: 15_000 });

    // Secrets nav should be present for admin
    await expect(page.getByRole("link", { name: "Secrets & Variables" })).toBeVisible();
  });
});

// --- Member flow tests ---

test.describe("member permissions", () => {
  test.beforeEach(async () => {
    await setRole("member");
  });

  test("member sees read-only general settings", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });

    // Wait for full page to render
    await expect(page.getByText("Danger Zone")).toBeVisible({ timeout: 15_000 });

    // Display name input should be disabled
    const displayNameInput = page.locator("input").first();
    await expect(displayNameInput).toHaveValue("Test Org");
    await expect(displayNameInput).toBeDisabled();

    // Delete button should be disabled
    const deleteBtn = page.getByRole("button", { name: "Delete" });
    await expect(deleteBtn).toBeDisabled();
  });

  test("member cannot see invite button", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });

    // Wait for members to load
    await expect(page.getByText("Other User")).toBeVisible({ timeout: 15_000 });

    // Invite button should not be present
    await expect(page.getByRole("button", { name: /invite members/i })).toBeHidden();
  });

  test("member cannot see Secrets & Variables nav", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });

    // Wait for page to render
    await expect(page.getByText("Danger Zone")).toBeVisible({ timeout: 15_000 });

    // Secrets nav should not be present
    await expect(page.getByText("Secrets & Variables")).toBeHidden();
  });

  test("member can see leave button", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });

    // Wait for full page
    await expect(page.getByText("Danger Zone")).toBeVisible({ timeout: 15_000 });

    const leaveBtn = page.getByRole("button", { name: "Leave" });
    await expect(leaveBtn).toBeVisible();
    await expect(leaveBtn).toBeEnabled();
  });
});
