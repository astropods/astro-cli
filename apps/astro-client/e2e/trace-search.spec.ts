import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const TRACES = "/testuser/agents/dep-slack-full-1/traces";

test.beforeEach(async () => {
  await resetMockBackend();
});

// The mock backend serves two traces: trace-1 ("chat completion") and
// trace-2 ("tool call"). The table shows the trace id, and search also matches
// the (non-visible) span name.
test("search filters the trace list by name and id", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(TRACES, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("trace-1")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("trace-2")).toBeVisible();

  const search = page.getByRole("textbox", { name: /search traces/i });

  // Matches trace-2's span name ("tool call").
  await search.fill("tool");
  await expect(page.getByText("trace-2")).toBeVisible();
  await expect(page.getByText("trace-1")).toHaveCount(0);

  // Matches trace-1's span name ("chat completion").
  await search.fill("chat");
  await expect(page.getByText("trace-1")).toBeVisible();
  await expect(page.getByText("trace-2")).toHaveCount(0);

  // Matches a trace id directly.
  await search.fill("trace-2");
  await expect(page.getByText("trace-2")).toBeVisible();
  await expect(page.getByText("trace-1")).toHaveCount(0);

  // Clearing the search restores the full list.
  await search.fill("");
  await expect(page.getByText("trace-1")).toBeVisible();
  await expect(page.getByText("trace-2")).toBeVisible();
});

test("search shows an empty state when nothing matches", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(TRACES, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("trace-1")).toBeVisible({ timeout: 20_000 });

  await page.getByRole("textbox", { name: /search traces/i }).fill("zzz-no-match");
  await expect(page.getByText("No traces found.")).toBeVisible();
});
