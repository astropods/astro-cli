import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const AGENT = "code-reviewer";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("breadcrumb account link: navigates from blueprint detail to account profile", async ({
  page,
}) => {
  test.setTimeout(30_000);

  // Step 1: land on a blueprint detail page
  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: AGENT })).toBeVisible({ timeout: 15_000 });

  // Step 2: click the account name in the breadcrumb → /testuser (account profile)
  // The breadcrumb renders two <a> elements with the same href (avatar + text link); pick the first.
  const breadcrumb = page.locator(`a[href="/${ACCOUNT}"]`).first();
  await expect(breadcrumb).toBeVisible({ timeout: 5_000 });
  await breadcrumb.click();
  await page.waitForURL(`**/${ACCOUNT}`, { timeout: 10_000 });

  // Step 3: confirm the profile page actually rendered — guards against silent
  // routing failures where the URL changes but the page doesn't load.
  // Use the account <h1> heading rather than the @handle text: during the React
  // Router transition, SidebarAuthor on the blueprint detail page still briefly
  // renders a second @testuser span, causing a Playwright strict-mode violation.
  await expect(page.getByRole("heading", { name: ACCOUNT })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("button", { name: /Blueprints/ })).toBeVisible();
});
