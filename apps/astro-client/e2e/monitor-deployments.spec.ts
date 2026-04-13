import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";

// Base URL for the deployed agent detail page
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("monitor tab: headline metrics are populated from observability summary", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}?tab=monitor`, { waitUntil: "domcontentloaded" });

  // Wait for the monitor tab content — "TOTAL REQUESTS" metric card
  await expect(page.getByText("TOTAL REQUESTS")).toBeVisible({ timeout: 15_000 });

  // Mock summary returns total_traces: 150 (use first() — "150" also appears in trace token column)
  await expect(page.getByText("150", { exact: true }).first()).toBeVisible({ timeout: 10_000 });

  // avg_latency_ms: 523 → rendered as "523ms" (use first() — also appears in trace latency column)
  await expect(page.getByText("523ms").first()).toBeVisible({ timeout: 5_000 });

  // error_rate: 0.02 → rendered as "2.0%"
  await expect(page.getByText("2.0%")).toBeVisible({ timeout: 5_000 });
});

test("monitor tab: trace rows are visible and clicking a row opens the detail panel", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}?tab=monitor`, { waitUntil: "domcontentloaded" });

  // Wait for trace table to load — external IDs (trace_id values) are shown in the ID column
  await expect(page.getByText("trace-1")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("trace-2")).toBeVisible({ timeout: 5_000 });

  // Click the "trace-1" ID cell — click bubbles up to the row's onClick handler
  await page.getByText("trace-1").click();

  // Side panel opens showing input and output from trace-1
  await expect(page.getByText("What is the weather today?")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/I don't have access to real-time weather data/i)).toBeVisible({ timeout: 5_000 });
});

test("deployments tab: stat cards and service accordion are visible", async ({ page }) => {
  test.setTimeout(30_000);
  // Default tab is deployments
  await page.goto(AGENT_DETAIL, { waitUntil: "domcontentloaded" });

  // Stat cards: build ID from mock is "build-123" → first 8 chars = "build-12"
  await expect(page.getByText("CURRENT BUILD")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("build-12")).toBeVisible({ timeout: 5_000 });

  // SERVICES stat card — dep-slack-full-1 has one workload, so value is "1"
  // exact: true avoids matching the "Services1" accordion section header
  await expect(page.getByText("SERVICES", { exact: true })).toBeVisible({ timeout: 5_000 });

  // The workload "agent" accordion should be auto-opened; target by title to avoid strict mode
  await expect(page.getByTitle("slack-config-full-agent")).toBeVisible({ timeout: 5_000 });
});

test("monitor tab: switching window keeps metrics visible", async ({ page }) => {
  // MonitorTab prefetches all 3 window queries on mount (not on window change).
  // Switching windows selects from already-cached data — no new network request fires.
  // This test verifies the UI switches cleanly without errors.
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}?tab=monitor`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("TOTAL REQUESTS")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("150", { exact: true }).first()).toBeVisible({ timeout: 10_000 });

  // Switch to "Last 1 hour" via the combobox
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "Last 1 hour" }).click();

  // Metrics remain visible after window switch (mock returns same data for all windows)
  await expect(page.getByText("TOTAL REQUESTS")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("150", { exact: true }).first()).toBeVisible({ timeout: 5_000 });
});

test("logs tab: log lines are loaded for the first container", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}?tab=logs`, { waitUntil: "domcontentloaded" });

  // Mock returns log lines for the workload's container
  await expect(page.getByText(/Starting agent server on :8080/)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(/Agent ready to accept requests/)).toBeVisible({ timeout: 5_000 });
});
