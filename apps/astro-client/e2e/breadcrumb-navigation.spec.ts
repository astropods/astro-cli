import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const AGENT = "code-reviewer";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("breadcrumb account link: blueprint cards are clickable after navigating from blueprint detail", async ({
  page,
}) => {
  test.setTimeout(30_000);

  // Step 1: land on a blueprint detail page
  await page.goto(`/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: AGENT })).toBeVisible({ timeout: 15_000 });

  // Step 2: click the account name in the breadcrumb → /blueprints?account=testuser
  // The breadcrumb renders two <a> elements with the same href (avatar + text link); pick the first.
  const breadcrumb = page.locator(`a[href="/blueprints?account=${ACCOUNT}"]`).first();
  await expect(breadcrumb).toBeVisible({ timeout: 5_000 });
  await breadcrumb.click();
  await page.waitForURL(`**/blueprints?account=${ACCOUNT}`, { timeout: 10_000 });

  // Step 3: wait for blueprint cards to render
  const firstCard = page.locator(`a[href^="/${ACCOUNT}/"]`).first();
  await expect(firstCard).toBeVisible({ timeout: 15_000 });

  // Step 4: click a blueprint card — should navigate to its detail page
  const targetHref = await firstCard.getAttribute("href");
  await Promise.all([
    page.waitForURL(`**${targetHref}`, { timeout: 10_000 }),
    firstCard.click(),
  ]);
});
