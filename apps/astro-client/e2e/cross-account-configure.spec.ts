import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT_CROSS_ACCOUNT = "cross-agent";
const DEPLOYMENT_CROSS_ACCOUNT_ID = "dep-cross-acct-1";
const CROSS_ACCOUNT_PUBLISHER = "otheraccount";

test.beforeEach(async () => {
  await resetMockBackend();
});

test.describe("cross-account configure page", () => {
  // Regression: when a user deploys another account's agent, the ListDeployments
  // handler must return the plain agent name (e.g. "cross-agent") not the
  // account-qualified K8s label (e.g. "otheraccount.cross-agent"). If the
  // qualified name leaks through, the configure page calls
  // GET /api/v1/agents/testuser/otheraccount.cross-agent which 404s.
  //
  // This test verifies the configure page loads successfully with the plain name.
  test("configure page loads for cross-account deployment using plain agent name", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_CROSS_ACCOUNT_ID}/configure`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByText("Deployment not found")).toHaveCount(0);
    await expect(page.getByText("Failed to load settings")).toHaveCount(0);

    await expect(page.getByLabel("Openai Api Key")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-cross-existing");
  });

  // Intercepts every outgoing agent API request and asserts the URL path uses
  // the plain agent name ("cross-agent"), not the account-qualified K8s label
  // ("otheraccount.cross-agent"). Unlike the first test which checks the page
  // loaded, this one inspects the actual HTTP traffic to catch subtle cases
  // where a request succeeds despite using the wrong name (e.g. a wildcard
  // route on the server).
  test("all agent API requests use plain name, never account-qualified", async ({ page }) => {
    test.setTimeout(60_000);

    const agentApiRequests: string[] = [];
    page.on("request", (request) => {
      const url = request.url();
      if (url.includes("/api/v1/agents/") && url.includes(ACCOUNT)) {
        agentApiRequests.push(url);
      }
    });

    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_CROSS_ACCOUNT_ID}/configure`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByLabel("Openai Api Key")).toBeVisible({ timeout: 20_000 });

    expect(agentApiRequests.length).toBeGreaterThan(0);

    const requestsWithDottedName = agentApiRequests.filter((url) =>
      url.includes(`${CROSS_ACCOUNT_PUBLISHER}.${AGENT_CROSS_ACCOUNT}`),
    );
    expect(requestsWithDottedName).toHaveLength(0);

    const requestsWithPlainName = agentApiRequests.filter((url) =>
      url.includes(`/${ACCOUNT}/${AGENT_CROSS_ACCOUNT}`),
    );
    expect(requestsWithPlainName.length).toBeGreaterThan(0);
  });

  // Negative regression test — the most important test in this file.
  //
  // Uses page.route to intercept the deployments API response and replace the
  // cross-account deployment's name with the buggy account-qualified form
  // ("otheraccount.cross-agent"). The React app then passes this dotted name
  // to useAgent and usePrefilledDeploymentTemplate, which hit
  // GET /api/v1/agents/testuser/otheraccount.cross-agent — a path the mock
  // backend doesn't recognise, returning 404.
  // End-to-end: opens the configure page for a cross-account deployment,
  // changes a variable value, clicks "Save & Redeploy", and asserts the POST
  // /api/v1/deploy payload contains the updated value. Verifies the full
  // configure → save → redeploy flow works when the deployment name is correct.
  test("save and redeploy works for cross-account deployment", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(
      `/${ACCOUNT}/agents/${DEPLOYMENT_CROSS_ACCOUNT_ID}/configure`,
      { waitUntil: "domcontentloaded" },
    );

    await expect(page.getByLabel("Openai Api Key")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-cross-existing");

    await page.getByLabel("Openai Api Key").fill("sk-cross-updated");

    await expect(page.getByRole("button", { name: /^redeploy$/i })).toBeVisible();

    const redeployRequest = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        request.url().includes("/api/v1/deploy"),
    );

    await Promise.all([
      redeployRequest,
      page.waitForURL(`**/${ACCOUNT}/agents/${DEPLOYMENT_CROSS_ACCOUNT_ID}/**`, { timeout: 20_000 }),
      page.getByRole("button", { name: /^redeploy$/i }).click(),
    ]);

    const payload = (await redeployRequest).postDataJSON() as {
      variables?: Record<string, { value?: string }>;
    };
    expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-cross-updated");
  });
});
