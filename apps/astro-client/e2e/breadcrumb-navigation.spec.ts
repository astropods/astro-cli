import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT = "code-reviewer";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("breadcrumb account link: navigates from blueprint detail to account profile", async ({
  page,
}) => {
  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();

  const breadcrumb = page.locator(`a[href="/${ACCOUNT}"]`).first();
  await expect(breadcrumb).toBeVisible();
  await breadcrumb.click();
  await page.waitForURL(`**/${ACCOUNT}`, { timeout: 20_000 });

  // Router transition can briefly leave two @handle spans in the tree — retry the profile heading.
  await expect(async () => {
    await expect(page.getByRole("heading", { name: ACCOUNT })).toBeVisible();
    await expect(page.getByRole("button", { name: /Blueprints/ })).toBeVisible();
  }).toPass({ timeout: 20_000 });
});
