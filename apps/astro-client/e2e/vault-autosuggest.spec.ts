import { expect, test, type APIRequestContext } from "@playwright/test";
import { MOCK_BACKEND, resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";

// The ingestion-scheduled blueprint has OPENAI_API_KEY as its only variable.
// The mock starts with an empty vault; this suite seeds its own entries in beforeEach.
const DEPLOY_PAGE = `/deploy/${ACCOUNT}/ingestion-scheduled`;

type VaultVar = { name: string; value: string; secret: boolean; description: string };

async function seedVault(request: APIRequestContext, variables: VaultVar[]) {
  await request.post(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/variables`, {
    data: { variables },
  });
}

test.describe("vault autosuggest", () => {
  test.beforeEach(async ({ request }) => {
    await resetMockBackend(request);
    // Seed the vault entries used by autosuggest tests. The mock starts with an
    // empty vault so other test suites are unaffected by these entries.
    await seedVault(request, [
      { name: "OPENAI_API_KEY", value: "sk-demo-existing", secret: true, description: "Primary OpenAI API key" },
      { name: "APP_ENV", value: "development", secret: false, description: "Runtime environment" },
    ]);
  });

  test("exact match auto-fills field and shows auto-fill label", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Field should have auto-filled with the vault ref chip instead of a text input
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toBeVisible({ timeout: 10_000 });
    await expect(chip).toContainText("OPENAI_API_KEY");

    // Auto-fill label with wand icon should be visible next to the chip
    await expect(page.getByText(/auto.filled/i)).toBeVisible({ timeout: 10_000 });
  });

  test("multiple matches show 'Auto-filled · N other match' label", async ({ page, request }) => {
    test.setTimeout(30_000);
    // Add a lowercase variant so OPENAI_API_KEY has 1 exact + 1 close match.
    await seedVault(request, [{ name: "openai_api_key", value: "sk-lower", secret: true, description: "" }]);

    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /clear vault reference/i })).toBeVisible({ timeout: 20_000 });

    // Auto-fill should have picked the exact-case entry and label should mention the other match
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toContainText("OPENAI_API_KEY");
    await expect(page.getByText(/auto.filled\s*·\s*1 other match/i)).toBeVisible({ timeout: 10_000 });
  });

  test("clicking 'N other match' label opens picker with exact and close match annotations", async ({ page, request }) => {
    test.setTimeout(30_000);
    // Exact + close match setup — same as the label test above.
    await seedVault(request, [{ name: "openai_api_key", value: "sk-lower", secret: true, description: "" }]);

    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    const labelBtn = page.getByText(/auto.filled\s*·\s*1 other match/i);
    await expect(labelBtn).toBeVisible({ timeout: 20_000 });

    // Click the label button to open the picker
    await labelBtn.click();
    await expect(page.getByPlaceholder("Find...")).toBeVisible();

    // Scope all row lookups to the picker popover to avoid matching the chip button
    const picker = page.locator('[data-radix-popper-content-wrapper]');

    // OPENAI_API_KEY row should show the exact match icon
    const exactRow = picker.getByRole("button").filter({ hasText: /^OPENAI_API_KEY/ });
    await expect(exactRow.locator('[aria-label="Exact match"]')).toBeVisible();

    // openai_api_key row should show the close match icon
    const closeRow = picker.getByRole("button").filter({ hasText: /^openai_api_key/ });
    await expect(closeRow.locator('[aria-label="Case insensitive match"]')).toBeVisible();

    // Select the close match entry
    await closeRow.click();

    // Auto-fill label should clear after explicit selection
    await expect(page.getByText(/auto.filled/i)).toHaveCount(0);
    // Chip should now show the close match entry
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toContainText("openai_api_key");
  });

  test("close match auto-fills when no exact match exists", async ({ page, request }) => {
    test.setTimeout(30_000);
    // Remove exact entry; only the lowercase variant remains → close match only.
    await request.delete(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/variables/OPENAI_API_KEY`);
    await seedVault(request, [{ name: "openai_api_key", value: "sk-lower", secret: true, description: "" }]);

    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Chip should auto-fill with the close match
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toBeVisible({ timeout: 10_000 });
    await expect(chip).toContainText("openai_api_key");

    // Auto-fill label should be present (single match, no "others")
    await expect(page.getByText(/auto.filled/i)).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(/other match/i)).toHaveCount(0);
  });

  test("clearing the auto-fill chip restores the text input", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toBeVisible({ timeout: 20_000 });

    // Hover to reveal the × button and click it
    await chip.hover();
    await chip.click();

    // Text input should reappear; auto-fill label should be gone
    await expect(page.getByLabel(/openai api key/i)).toBeVisible();
    await expect(page.getByText(/auto.filled/i)).toHaveCount(0);
  });

  test("explicit picker selection clears auto-fill label", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /clear vault reference/i })).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/auto.filled/i)).toBeVisible({ timeout: 10_000 });

    // Open the vault picker and select APP_ENV (a different entry)
    await page.getByTitle("Insert vault reference").click();
    await expect(page.getByPlaceholder("Find...")).toBeVisible();
    await page.getByRole("button").filter({ hasText: /^APP_ENV/ }).click();

    // Auto-fill label should be gone — user made an explicit choice
    await expect(page.getByText(/auto.filled/i)).toHaveCount(0);
    // Chip should now show the newly selected entry
    const chip = page.getByRole("button", { name: /clear vault reference/i });
    await expect(chip).toContainText("APP_ENV");
  });

  test("picker marks exact match with tooltip 'Exact match'", async ({ page }) => {
    test.setTimeout(30_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    // Wait for the vault list to resolve before opening the picker — the auto-fill
    // chip only renders once entries are loaded, so it doubles as a load gate.
    // Without this, clicking too early opens the picker in its "No variables yet"
    // empty state and the Find... input never appears.
    await expect(page.getByRole("button", { name: /clear vault reference/i })).toBeVisible({ timeout: 20_000 });

    // Open the picker
    await page.getByTitle("Insert vault reference").click();
    await expect(page.getByPlaceholder("Find...")).toBeVisible();

    // Hover the match icon next to OPENAI_API_KEY to trigger tooltip
    const listItem = page.getByRole("button").filter({ hasText: /^OPENAI_API_KEY/ }).last();
    await listItem.locator("span.cursor-default").first().hover();
    await expect(page.getByText("Exact match")).toBeVisible({ timeout: 5_000 });
  });

  test("picker marks case-insensitive entry with tooltip 'Case insensitive match'", async ({ page, request }) => {
    test.setTimeout(30_000);
    // Remove the exact-case vault entry and add a lowercase variant so the blueprint
    // variable OPENAI_API_KEY only gets a case-insensitive (close) match.
    await request.delete(`${MOCK_BACKEND}/api/v1/accounts/${ACCOUNT}/variables/OPENAI_API_KEY`);
    await seedVault(request, [{ name: "openai_api_key", value: "sk-lower", secret: true, description: "" }]);

    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    // Wait for the vault list to resolve before opening the picker — the close-match
    // auto-fill chip only renders once entries are loaded.
    await expect(page.getByRole("button", { name: /clear vault reference/i })).toBeVisible({ timeout: 20_000 });

    // Open the picker
    await page.getByTitle("Insert vault reference").click();
    await expect(page.getByPlaceholder("Find...")).toBeVisible();

    // Hover the match icon next to openai_api_key
    const listItem = page.getByRole("button").filter({ hasText: /^openai_api_key/ }).last();
    await listItem.locator("span.cursor-default").first().hover();
    await expect(page.getByText("Case insensitive match")).toBeVisible({ timeout: 5_000 });
  });

  test("deploy payload uses vault ref when field was auto-filled", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(DEPLOY_PAGE, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Verify auto-fill kicked in
    await expect(page.getByRole("button", { name: /clear vault reference/i })).toBeVisible({ timeout: 10_000 });

    // Fill in the required schedule to enable deploy
    await page.getByRole("combobox").filter({ hasText: "Select a schedule" }).click();
    await page.getByRole("option", { name: "Daily at midnight" }).click();

    const deployRequest = page.waitForRequest(
      (req) => req.method() === "POST" && req.url().includes("/api/v1/deploy"),
    );
    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);

    const payload = (await deployRequest).postDataJSON() as {
      variables?: Record<string, { value?: string; ref?: string }>;
    };
    // Vault ref should be submitted as ref: "OPENAI_API_KEY", not as a plain value
    expect(payload.variables?.OPENAI_API_KEY?.ref).toBe("OPENAI_API_KEY");
    expect(payload.variables?.OPENAI_API_KEY?.value).toBeUndefined();
  });
});
