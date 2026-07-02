import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const DEPLOYMENT_KNOWLEDGE_BINDINGS_ID = "dep-knowledge-bindings-1";
const CONFIGURE_PAGE = `/${ACCOUNT}/agents/${DEPLOYMENT_KNOWLEDGE_BINDINGS_ID}/configure`;

test.beforeEach(async ({ request }) => {
  await resetMockBackend(request);
});

test("configure blocks redeploy when Shared knowledge has no selected store", async ({ page }) => {
  test.setTimeout(90_000);
  let deployPosts = 0;

  await page.route("**/api/v1/deploy", async (route) => {
    if (route.request().method() === "POST") deployPosts++;
    await route.continue();
  });

  await page.goto(CONFIGURE_PAGE, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Knowledge", exact: true })).toBeVisible({ timeout: 20_000 });

  await page.getByRole("radio", { name: "Shared", exact: true }).click();
  await expect(page.getByText("Redeploy to apply 1 change.")).toBeVisible();

  await page.getByRole("button", { name: "Redeploy", exact: true }).click();

  await expect(page.getByText("Select a shared knowledge store or switch to Local.")).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`${DEPLOYMENT_KNOWLEDGE_BINDINGS_ID}/configure$`));
  expect(deployPosts).toBe(0);
});
