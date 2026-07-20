import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const DEPLOYMENT_ID = "dep-slack-full-1";
const AGENT_DETAIL = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}`;

test.beforeEach(async () => {
  await resetMockBackend();
});

test("monitor tab: charts and headings are populated from observability metrics", async ({ page }) => {
  await page.goto(`${AGENT_DETAIL}/monitor`, { waitUntil: "domcontentloaded" });

  await expect(async () => {
    await expect(page.getByText("Token Usage")).toBeVisible();
    await expect(page.getByText("Requests & Latency")).toBeVisible();
  }).toPass();
});

test("traces tab: trace rows are visible and clicking a row opens the detail panel", async ({ page }) => {
  await page.goto(`${AGENT_DETAIL}/traces`, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("trace-1")).toBeVisible();
  await expect(page.getByText("trace-2")).toBeVisible();

  await page.getByText("trace-1").click();

  await expect(page.getByText("What is the weather today?")).toBeVisible();
  await expect(page.getByText(/I don't have access to real-time weather data/i)).toBeVisible();
});

test("deployments tab: pod tile is visible for the active workload", async ({ page }) => {
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  await expect(async () => {
    await expect(page.getByText("agent", { exact: true })).toBeVisible();
    await expect(page.getByText("Online")).toBeVisible();
  }).toPass();
});

test("monitor tab: switching range keeps charts visible", async ({ page }) => {
  await page.goto(`${AGENT_DETAIL}/monitor`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Token Usage")).toBeVisible();

  await expect(page.getByRole("button", { name: "7D" })).toHaveAttribute("aria-pressed", "true");

  await page.getByRole("button", { name: "14D" }).click();
  await expect(page.getByRole("button", { name: "14D" })).toHaveAttribute("aria-pressed", "true");

  await expect(page.getByText("Token Usage")).toBeVisible();
  await expect(page.getByText("Requests & Latency")).toBeVisible();
});

test("deployments tab: clicking a pod tile opens detail panel with logs", async ({ page }) => {
  await page.goto(`${AGENT_DETAIL}/deployments`, { waitUntil: "domcontentloaded" });

  const agentTile = page.getByText("agent", { exact: true });
  await expect(agentTile).toBeVisible();
  await agentTile.click();

  await expect(page.getByText("General")).toBeVisible();

  // Use the tab's role rather than a bare text match: the pod detail panel can
  // now render an "Errors in logs" banner whose text also contains "Logs".
  await page.getByRole("button", { name: "Logs" }).click();

  await expect(async () => {
    await expect(page.getByText(/Starting agent server on :8080/)).toBeVisible();
    await expect(page.getByText(/Agent ready to accept requests/)).toBeVisible();
  }).toPass();
});
