import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const MONITOR = "/testuser/agents/dep-slack-full-1/monitor";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("share button copies a deep link to the open trace", async ({ page, context }) => {
  test.setTimeout(60_000);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(MONITOR, { waitUntil: "domcontentloaded" });

  await page.getByText("trace-1").click();
  await expect(page.getByRole("dialog", { name: "Trace details" })).toBeVisible({ timeout: 20_000 });
  await expect(page).toHaveURL(/trace=trace-1/);

  await page.getByRole("button", { name: "Copy link to this trace" }).click();
  await expect(page.getByText("Link copied")).toBeVisible({ timeout: 10_000 });

  // The copied link is the current deep link, so it reopens the same trace.
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain("trace=trace-1");
});

test("deep link opens the trace panel and preserves the selected time window", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`${MONITOR}?trace=trace-2&window=30d`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("dialog", { name: "Trace details" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: "30D" })).toHaveAttribute("aria-pressed", "true");
});

test("changing the time range writes it to the URL so the link is shareable", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(MONITOR, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Token Usage")).toBeVisible({ timeout: 20_000 });

  await page.getByRole("button", { name: "14D" }).click();
  await expect(page).toHaveURL(/window=14d/);
});
