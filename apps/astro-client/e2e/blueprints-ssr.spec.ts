import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const PERSONAL_ACCOUNT = "testuser";
const ORG_ACCOUNT = "test-org";
const ORG_DISPLAY_NAME = "Test Org";

/*
 * Tests in this file demonstrate three concerns with the SSR loader + skeleton
 * cards introduced on the blueprints page:
 *
 *   1. When the loader's fallback path is hit (unauthenticated / error), it
 *      returns { count: 0 }, which renders an empty loading grid with zero
 *      skeleton cards. The container is in the DOM but has no children, so
 *      it has zero height — the user sees a completely blank page where the
 *      old code showed a spinner.
 *   2. The loader fetches the full blueprint list just to use `.count`. The
 *      `agents` array is embedded in the SSR HTML and then thrown away; the
 *      client query refetches the same data. Two round-trips for the same
 *      payload on every cold load.
 *   3. The loader always uses the PERSONAL account. When the user's active
 *      scope is an org (stored in calStorage), skeleton count reflects the
 *      personal account, not the active one — skeletons flash at a count
 *      that has nothing to do with what's about to load.
 *
 * Each test is written to FAIL against the PR as-is, as evidence of the issue.
 */

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

/*
 * Concern 2: the loader fetches the full blueprint list just to read .count,
 * and the client query refetches the same data anyway.
 *
 * We inspect the SSR HTML for the agent names that only appear if the loader
 * embedded the full list in the response. We then capture the client-side
 * GET to `/api/v1/agents/<account>` — if the PR wired `loaderData` into the
 * query cache as `initialData`, this request wouldn't fire on cold load.
 */
test("SSR HTML leaks the full agents array, and the client refetches the same endpoint", async ({ page, request }) => {
  test.fail(); // known issue: loader serializes full agents array into SSR HTML and client refetches same endpoint
  test.setTimeout(30_000);

  const ssrResponse = await request.get("/blueprints");
  expect(ssrResponse.status()).toBe(200);
  const html = await ssrResponse.text();

  /*
 * Agent names only present in the full BlueprintsListResponse payload.
   * If the loader called a dedicated count endpoint (or threw the agents
   * array away before returning), these would not appear in the HTML.
   *
   * Assertion is inverted — we want the test to FAIL under current code to
   * demonstrate that the list is being serialised into the HTML payload
   * only to be discarded.
   */
  expect(html, "SSR HTML should not contain the full blueprint list").not.toMatch(/code-reviewer|slack-config-full|slack-overlap-targets/);

  const clientListRequests: string[] = [];
  page.on("request", (req) => {
    const url = req.url();
    if (/\/api\/v1\/agents\/[^/?]+$/.test(url) && req.method() === "GET") {
      clientListRequests.push(url);
    }
  });

  await page.goto("/blueprints", { waitUntil: "networkidle" });

  const clientFetchedPersonal = clientListRequests.some((url) =>
    url.endsWith(`/api/v1/agents/${PERSONAL_ACCOUNT}`),
  );
  expect(clientFetchedPersonal, "client refetched the sameist the loader already returned").toBe(false);
});

/*
 * Concern 3: skeleton count reflects the PERSONAL account, not the active
 * scope.
 *
 * `getPersonalAccount()` hardcodes `type === "personal"`. If the user's
 * active scope (from localStorage) is an org with a different blueprint
 * count, the skeleton count is always for the wrong account.
 *
 * Mock setup: testuser (personal) has 5 blueprints, test-org has 0. With
 * active scope = test-org, the user should see a short loading state for
 * the org and then the empty state. Instead they see 5 skeletons flash
 * before the empty state — based on personal's count, not the org's.
 */
test("skeleton count reflects personal account when active scope is an org", async ({ page, context }) => {
  test.setTimeout(30_000);

  await context.addInitScript((org) => {
    try { localStorage.setItem("astro:default-account", org); } catch { /* noop */ }
  }, ORG_ACCOUNT);

  /*
   * Hold the org query open so the loading state persists long enough to
   * assertn. Without this the mock returns instantly and the skeletons
   * swap out for the empty state before Playwright can count them.
   */
  await page.route(`**/api/v1/agents/${ORG_ACCOUNT}`, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 5_000));
    await route.continue();
  });

  await page.goto("/blueprints", { waitUntil: "domcontentloaded" });

  const loadingGrid = page.getByRole("status", { name: "Loading blueprints" });
  const skeletonCards = loadingGrid.locator(".animate-pulse");

  /*
   * Wait until the active scope has hydrated to the org. Before this, the
   * skeletons seen in the HTML are just the SSR render (pre-hydration); we
   * want to assert on the state *after* hydration, where the query is
   * known to be for the org.
   */
  const scopeSwitcher = page.getByRole("button", { name: "Switch account" });
  await expect(scopeSwitcher).toContainText(ORG_DISPLAY_NAME, { timeout: 10_000 });

  /*
   * Active scope has 0 blueprints; personal has 5. Correct behavior would
   * be to render skeletons for the active scope (0, or the default 6).
   * Under current code the skeleton count comes from personal = 5, which
   * is what the user is about to *not* see. Assertion is inverted so the
   * test fails under current code.
   */
  await expect(skeletonCards, "skeletons should not be based on personal account's count").not.toHaveCount(5);
});
