import { expect, test } from "@playwright/test";
import { MOCK_BACKEND, resetMockBackend } from "./helpers";

async function setRole(role: string) {
  const res = await fetch(`${MOCK_BACKEND}/test/set-role`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ role }),
  });
  if (!res.ok) throw new Error(`set-role failed: ${res.status}`);
}

test.beforeEach(async () => {
  await resetMockBackend();
});

test.describe("admin permissions", () => {
  test.beforeEach(async () => {
    await setRole("admin");
  });

  test("admin can see and edit org general settings", async ({ page }) => {
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Danger Zone")).toBeVisible();

    const displayNameInput = page.locator("input").first();
    await expect(displayNameInput).toHaveValue("Test Org");
    await expect(displayNameInput).toBeEnabled();

    await expect(page.getByRole("button", { name: "Delete" })).toBeEnabled();
  });

  test("admin sees invite button on members page", async ({ page }) => {
    await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /invite members/i })).toBeVisible();
  });

  test("admin sees Variables & Secrets nav", async ({ page }) => {
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Danger Zone")).toBeVisible();
    await expect(page.getByRole("link", { name: "Variables & Secrets" })).toBeVisible();
  });
});

test.describe("member permissions", () => {
  test.beforeEach(async () => {
    await setRole("member");
  });

  test("member sees read-only general settings", async ({ page }) => {
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Danger Zone")).toBeVisible();

    const displayNameInput = page.locator("input").first();
    await expect(displayNameInput).toHaveValue("Test Org");
    await expect(displayNameInput).toBeDisabled();
    await expect(page.getByRole("button", { name: "Delete" })).toBeDisabled();
  });

  test("member cannot see invite button", async ({ page }) => {
    await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Other User")).toBeVisible();
    await expect(page.getByRole("button", { name: /invite members/i })).toBeHidden();
  });

  test("member cannot see Variables & Secrets nav", async ({ page }) => {
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Danger Zone")).toBeVisible();
    await expect(page.getByText("Variables & Secrets")).toBeHidden();
  });

  test("member can see leave button", async ({ page }) => {
    await page.goto("/settings/org/test-org/general", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Danger Zone")).toBeVisible();

    const leaveBtn = page.getByRole("button", { name: "Leave" });
    await expect(leaveBtn).toBeVisible();
    await expect(leaveBtn).toBeEnabled();
  });
});

test.describe("organizations list role badges", () => {
  test("shows Admin badge next to org name when role is admin", async ({ page }) => {
    await setRole("admin");
    await page.goto("/settings/organizations", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Test Org")).toBeVisible();
    await expect(page.getByText("Admin")).toBeVisible();
  });

  test("shows Member badge next to org name when role is member", async ({ page }) => {
    await setRole("member");
    await page.goto("/settings/organizations", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Test Org")).toBeVisible();
    await expect(page.getByText("Member")).toBeVisible();
  });

  test("shows Owner badge next to org name when role is owner", async ({ page }) => {
    await setRole("owner");
    await page.goto("/settings/organizations", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Test Org")).toBeVisible();
    await expect(page.getByText("Owner")).toBeVisible();
  });
});
