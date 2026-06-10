import { expect, test } from "@playwright/test";
import { clickMenuItem, resetMockBackend } from "./helpers";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("admin can invite a new member by email", async ({ page }) => {
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: /invite members/i })).toBeVisible();

  await page.getByRole("button", { name: /invite members/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible();

  await page.getByPlaceholder("Enter email or username").fill("newuser@example.com");
  await expect(page.getByRole("option", { name: /invite.*by email/i })).toBeVisible();
  await page.getByRole("option", { name: /invite.*by email/i }).click();

  await expect(page.getByRole("button", { name: /send invitation/i })).toBeEnabled();

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
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Other User")).toBeVisible();

  const removeReq = page.waitForRequest(
    (req) => req.method() === "DELETE" && req.url().includes("/members/"),
  );

  const otherUserRow = page.locator("tbody tr").nth(1);
  await expect(otherUserRow).toContainText("Other User");
  await clickMenuItem(page, () => otherUserRow.locator("button").last().click(), /remove member/i);

  const req = await removeReq;
  expect(req.url()).toContain("/api/v1/accounts/test-org/members/user-2");
});

test("admin can change a member's role", async ({ page }) => {
  await page.goto("/settings/org/test-org/members", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Other User")).toBeVisible();

  const roleReq = page.waitForRequest(
    (req) => req.method() === "PUT" && req.url().includes("/members/"),
  );

  const otherUserRow = page.locator("tbody tr").nth(1);
  await expect(otherUserRow).toContainText("Other User");
  await otherUserRow.getByRole("button", { name: /member/i }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await page.getByRole("menuitem", { name: /admin/i }).click();

  const req = await roleReq;
  expect(req.url()).toContain("/api/v1/accounts/test-org/members/user-2");
  const body = req.postDataJSON() as { role: string };
  expect(body.role).toBe("admin");
});
