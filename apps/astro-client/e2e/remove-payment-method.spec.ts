import { expect, test } from "@playwright/test";
import { MOCK_BACKEND, resetMockBackend } from "./helpers";

test.beforeEach(async () => {
  await resetMockBackend();
});

async function setOwedBalance(owed: boolean) {
  const res = await fetch(`${MOCK_BACKEND}/test/set-billing-owed`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ owed }),
  });
  if (!res.ok) throw new Error(`set-billing-owed failed: ${res.status}`);
}

async function gotoBilling(page: import("@playwright/test").Page) {
  await page.goto("/settings/billing", { waitUntil: "domcontentloaded" });
  await expect(page.getByText(/•••• 4242/)).toBeVisible({ timeout: 15_000 });
}

// One click used to detach the card outright. The card paying for running
// agents is not something to remove without saying so.
test("removing a card asks for confirmation first", async ({ page }) => {
  test.setTimeout(45_000);
  await gotoBilling(page);

  await page.getByRole("button", { name: "Remove" }).click();

  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog")).toContainText(/running agents stop/i);
  // Still attached: the dialog is a gate, not a receipt.
  await expect(page.getByText(/•••• 4242/)).toBeVisible();
});

// The server refuses with 409 and explains why. Swallowing that into a generic
// toast is what let the old flow look like a transient failure.
test("an outstanding balance blocks removal and says so", async ({ page }) => {
  test.setTimeout(45_000);
  await setOwedBalance(true);
  await gotoBilling(page);

  await page.getByRole("button", { name: "Remove" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("checkbox").check();
  await dialog.getByRole("button", { name: /Remove payment method/i }).click();

  await expect(dialog).toContainText(/outstanding balance/i, { timeout: 15_000 });
  await expect(page.getByText(/•••• 4242/)).toBeVisible();
});

test("with nothing owed the card is removed", async ({ page }) => {
  test.setTimeout(45_000);
  await gotoBilling(page);

  await page.getByRole("button", { name: "Remove" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("checkbox").check();
  await dialog.getByRole("button", { name: /Remove payment method/i }).click();

  await expect(page.getByText(/No payment method on file/i)).toBeVisible({
    timeout: 15_000,
  });
});
