import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";

// Mock data: current user is "Test User" (user-1, admin), second member is "Other User" (user-2, member).
// Test User row has no action button (isCurrentUser = true).
// Other User row has a role dropdown button and a MoreHorizontal action button.

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("admin can invite a new member by email", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: /invite members/i })).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: /invite members/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });

  // Type a valid email — the InviteInput combobox shows a dropdown option to invite by email
  await page.getByPlaceholder("Enter email or username").fill("newuser@example.com");
  await expect(page.getByRole("option", { name: /invite.*by email/i })).toBeVisible({ timeout: 5_000 });
  await page.getByRole("option", { name: /invite.*by email/i }).click();

  // "Send invitation" button activates once at least one valid entry is present
  await expect(page.getByRole("button", { name: /send invitation/i })).toBeEnabled({ timeout: 5_000 });

  // Capture the invitation request and verify the payload
  const inviteReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes("/invitations"),
  );

  await page.getByRole("button", { name: /send invitation/i }).click();

  const req = await inviteReq;
  const body = req.postDataJSON() as { invitations: { value: string; kind: string; role: string }[] };
  expect(body.invitations).toHaveLength(1);
  expect(body.invitations[0].value).toBe("newuser@example.com");
  expect(body.invitations[0].kind).toBe("email");
  expect(body.invitations[0].role).toBe("member");
});

test("admin can remove a member via the action menu", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Other User")).toBeVisible({ timeout: 15_000 });

  // Capture the remove request before clicking (waitForRequest must be set up first)
  const removeReq = page.waitForRequest(
    (req) => req.method() === "DELETE" && req.url().includes("/members/"),
  );

  // Other User is the third direct child of bg-surface (nth(0) is a separator/header, nth(1) is Test User).
  // That row has two buttons: role dropdown ("Member") and action menu (icon-only MoreHorizontal).
  // Clicking the last button opens the action dropdown.
  const otherUserRow = page.locator("div.bg-surface > div").nth(2);
  await expect(otherUserRow).toContainText("Other User");
  await otherUserRow.locator("button").last().click();

  await page.getByRole("menuitem", { name: /remove member/i }).click();

  // Verify the DELETE was sent to the correct endpoint
  const req = await removeReq;
  expect(req.url()).toContain("/api/v1/accounts/test-org/members/user-2");
});

test("admin can change a member's role", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Other User")).toBeVisible({ timeout: 15_000 });

  // Capture the role-update request
  const roleReq = page.waitForRequest(
    (req) => req.method() === "PUT" && req.url().includes("/members/"),
  );

  // The role cell for Other User renders as a dropdown button with label "Member".
  // It is the first (and only) button in that row that has text content.
  const otherUserRow = page.locator("div.bg-surface > div").nth(2);
  await expect(otherUserRow).toContainText("Other User");
  await otherUserRow.getByRole("button", { name: /member/i }).click();

  // Dropdown opens — select "Admin"
  await page.getByRole("menuitem", { name: /admin/i }).click();

  // Verify the PUT was sent with the updated role
  const req = await roleReq;
  expect(req.url()).toContain("/api/v1/accounts/test-org/members/user-2");
  const body = req.postDataJSON() as { role: string };
  expect(body.role).toBe("admin");
});
