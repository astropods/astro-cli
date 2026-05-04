import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";

// Base URL for the deployed agent detail page
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("monitor tab: charts and headings are populated from observability metrics", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/monitor`, { waitUntil: "domcontentloaded" });

  // Wait for the monitor page sections
  await expect(page.getByText("Token Usage")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("Requests & Latency")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("heading", { name: "Traces" })).toBeVisible({ timeout: 5_000 });
});

test("monitor tab: trace rows are visible and clicking a row opens the detail panel", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/monitor`, { waitUntil: "domcontentloaded" });

  // Wait for trace table to load — external IDs (trace_id values) are shown in the ID column
  await expect(page.getByText("trace-1")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("trace-2")).toBeVisible({ timeout: 5_000 });

  // Click the "trace-1" ID cell — click bubbles up to the row's onClick handler
  await page.getByText("trace-1").click();

  // Side panel opens showing input and output from trace-1
  await expect(page.getByText("What is the weather today?")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/I don't have access to real-time weather data/i)).toBeVisible({ timeout: 5_000 });
});

test("deployments tab: pod tile is visible for the active workload", async ({ page }) => {
  test.setTimeout(30_000);
  // The index route redirects to /deployments
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  // The deployment has one workload: "slack-config-full-agent" (component: "agent")
  // Pod tile should show the component name and status
  await expect(page.getByText("agent", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("Online")).toBeVisible({ timeout: 5_000 });
});

test("monitor tab: switching range keeps charts visible", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/monitor`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Token Usage")).toBeVisible({ timeout: 15_000 });

  // The 7D button should be active by default
  await expect(page.getByRole("button", { name: "7D" })).toHaveAttribute("aria-pressed", "true");

  // Switch to 14D range
  await page.getByRole("button", { name: "14D" }).click();
  await expect(page.getByRole("button", { name: "14D" })).toHaveAttribute("aria-pressed", "true");

  // Charts remain visible after range switch
  await expect(page.getByText("Token Usage")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Requests & Latency")).toBeVisible({ timeout: 5_000 });
});

test("deployments tab: clicking a pod tile opens detail panel with logs", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  // Click the pod tile for the "agent" workload
  await expect(page.getByText("agent", { exact: true })).toBeVisible({ timeout: 15_000 });
  await page.getByText("agent", { exact: true }).click();

  // Pod detail panel opens with tabs
  await expect(page.getByText("General")).toBeVisible({ timeout: 5_000 });

  // Switch to Logs tab
  await page.getByText("Logs").click();

  // Mock returns log lines for the workload's container
  await expect(page.getByText(/Starting agent server on :8080/)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(/Agent ready to accept requests/)).toBeVisible({ timeout: 5_000 });
});
