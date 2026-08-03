import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Issue #1627: when an in-progress GitHub build finished, its card vanished and
// the finished build did not appear as an available upgrade until a refresh.

const ACCOUNT = "testuser";
const AGENT = "slack-overlap-targets";
const DEPLOYMENT_ID = "dep-slack-overlap-1";
const DEPLOYMENTS = `/${ACCOUNT}/agents/${DEPLOYMENT_ID}/deployments`;

const HISTORY_ROUTE = /\/deployment\/history(\?|$)/;
const GITHUB_ROUTE = new RegExp(`/agents/${ACCOUNT}/${AGENT}/github(\\?|$)`);

// GitHub-sourced record whose deployed build matches the latest published build,
// so the blueprint-versions upgrade path stays silent and only a finished GitHub
// build can raise the nudge.
function historyBody() {
  return {
    deployments: [
      {
        id: DEPLOYMENT_ID,
        agent_name: AGENT,
        revision: 1,
        build_id: "build-123",
        namespace: "astro-namespace",
        display_name: "Slack Overlap Bot",
        is_current: true,
        status: "healthy",
        deployed_at: "2026-07-01T00:00:00Z",
        spec: {},
        source: "github",
        branch: "main",
      },
    ],
    count: 1,
  };
}

// buildId "build-999" differs from the deployed "build-123" (an upgrade);
// "build-123" matches the deployed build (nothing newer -> on latest).
function githubBody(status: "building" | "registered", buildId = "build-999") {
  return {
    connected: true,
    repo_full_name: "acme/slack-overlap-targets",
    branch: "main",
    builds: [
      {
        id: "build-run-1",
        build_id: buildId,
        commit_sha: "9ec6558bfeed",
        branch: "main",
        status,
        commit_message: "fix: stream session creation to avoid a race",
        enqueued_at: "2026-07-14T00:00:00Z",
      },
    ],
  };
}

function routeJson(page: Page, re: RegExp, body: () => unknown) {
  return page.route(re, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body()) }),
  );
}

test.beforeEach(async () => {
  await resetMockBackend();
});

test("a finished build stays visible as an available upgrade without a refresh (#1627)", async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000);

  let phase: "building" | "available" | "latest" = "building";
  const github = () =>
    phase === "building"
      ? githubBody("building")
      : phase === "available"
        ? githubBody("registered")
        : githubBody("registered", "build-123");
  await routeJson(page, HISTORY_ROUTE, historyBody);
  await routeJson(page, GITHUB_ROUTE, github);

  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  const panel = page
    .locator("div.rounded-md", { has: page.getByRole("heading", { name: "Deployment History" }) })
    .first();

  // Phase 1: build in flight, so the panel shows the live building card.
  await expect(page.getByText("Pushing new build")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("New build available")).toHaveCount(0);
  await panel.screenshot({ path: testInfo.outputPath("01-building.png") });

  // Build finishes. No manual refresh: the panel polls, so it must swap the
  // building card for the "new build available" nudge on its own.
  phase = "available";

  await expect(page.getByText("New build available")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("Pushing new build")).toHaveCount(0);
  await panel.screenshot({ path: testInfo.outputPath("02-finished-available.png") });

  // Once the deployed build is the newest, the nudge clears and the header
  // shows a "Latest build" badge rather than falling silent (#1627 design).
  phase = "latest";

  await expect(page.getByText("Latest build")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("New build available")).toHaveCount(0);
  await panel.screenshot({ path: testInfo.outputPath("03-on-latest.png") });
});
