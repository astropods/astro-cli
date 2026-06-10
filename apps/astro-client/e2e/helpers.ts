import { expect, type APIRequestContext } from "@playwright/test";

/** Mock API — always 127.0.0.1; `localhost` can resolve to ::1 and miss the server. */
export const MOCK_BACKEND = "http://127.0.0.1:48787";

/** Reset mock backend state — throws if the server is not ready. */
export async function resetMockBackend(
  request?: APIRequestContext,
): Promise<void> {
  if (request) {
    const res = await request.post(`${MOCK_BACKEND}/test/reset`);
    if (!res.ok()) {
      throw new Error(`mock reset failed: ${res.status()} ${res.statusText()}`);
    }
    return;
  }

  const res = await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
  if (!res.ok) {
    throw new Error(`mock reset failed: ${res.status} ${res.statusText}`);
  }
}

/**
 * Assert UI that appears after debounced API calls (name checks, search, etc.).
 * Retries until timeout instead of a single fixed wait.
 */
export async function expectEventually(
  assertion: () => Promise<void>,
  timeout = 20_000,
): Promise<void> {
  await expect(assertion).toPass({ timeout });
}

/** Open a Radix/dropdown menu and wait until it is visible before clicking items. */
export async function clickMenuItem(
  page: import("@playwright/test").Page,
  openMenu: () => Promise<void>,
  itemName: RegExp | string,
): Promise<void> {
  await openMenu();
  const menu = page.getByRole("menu");
  await expect(menu).toBeVisible();
  await page.getByRole("menuitem", { name: itemName }).click();
}
