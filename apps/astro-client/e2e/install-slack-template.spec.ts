import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const AGENT_APP_TOKEN_ONLY = "code-reviewer";
const AGENT_SLACK_FULL = "slack-config-full";
const AGENT_SLACK_OVERLAP = "slack-overlap-targets";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";

// Guards against over-injecting Slack variables: when template only requires app token,
// deploy payload must not include a synthetic/empty bot token.
test("app-token-only template does not inject SLACK_BOT_TOKEN", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_APP_TOKEN_ONLY}`, { waitUntil: "domcontentloaded" });

  await expect(page).toHaveURL(new RegExp(`/deploy/${ACCOUNT}/${AGENT_APP_TOKEN_ONLY}$`));
  await expect(page.getByText("Agent not found")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByLabel("Openai Api Key")).toBeVisible();

  await page.getByRole("button", { name: /slack/i }).click();

  await expect(page.getByLabel("Slack App Token")).toBeVisible();
  await expect(page.getByLabel("Slack Bot Token")).toHaveCount(0);

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByLabel("Slack App Token").fill("xapp-test-value");

  const deployRequest = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.waitForURL("**/agents", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  const request = await deployRequest;
  const payload = request.postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };

  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-test-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-test-value");
  expect(payload.variables?.SLACK_BOT_TOKEN).toBeUndefined();

  // Let post-submit data loading settle before test teardown to avoid noisy aborted renders.
  await page.waitForLoadState("networkidle");
});

// Verifies happy-path mapping for full Slack config: required secrets + optional reactions
// are captured from UI and serialized into deploy payload.
test("full slack template sends reactions when provided", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /slack/i }).click();

  await expect(page.getByLabel("Slack Bot Token")).toBeVisible();
  await expect(page.getByLabel("Slack App Token")).toBeVisible();
  await expect(page.getByLabel("Actionable Reactions")).toBeVisible();
  await expect(page.getByLabel("Allowed Channel IDs")).toBeVisible();
  await expect(page.getByLabel("Allowed User IDs")).toBeVisible();

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByLabel("Slack Bot Token").fill("xoxb-test-value");
  await page.getByLabel("Slack App Token").fill("xapp-test-value");
  await page.getByLabel("Actionable Reactions").fill("ticket, bug");
  await page.getByLabel("Allowed Channel IDs").fill("C123, C999");
  await page.getByLabel("Allowed User IDs").fill("U123, U999");

  const deployRequest = page.waitForRequest((request) =>
    request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.waitForURL("**/agents", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  const payload = (await deployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-test-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-test-value");
  expect(payload.variables?.SLACK_ACTIONABLE_REACTIONS?.value).toBe("ticket, bug");
  expect(payload.variables?.SLACK_ALLOWED_CHANNEL_IDS?.value).toBe("C123, C999");
  expect(payload.variables?.SLACK_ALLOWED_USER_IDS?.value).toBe("U123, U999");
});

// Prevents regressions where optional fields accidentally become required and block launch.
// Empty optional reactions should still allow deploy.
test("optional actionable reactions can be omitted without blocking deploy", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /slack/i }).click();

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByLabel("Slack Bot Token").fill("xoxb-test-value");
  await page.getByLabel("Slack App Token").fill("xapp-test-value");
  // Leave Actionable Reactions empty on purpose (optional).

  const deployRequest = page.waitForRequest((request) =>
    request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.waitForURL("**/agents", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  const payload = (await deployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-test-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-test-value");
  expect(payload.variables?.SLACK_ACTIONABLE_REACTIONS?.value ?? "").toBe("");
  expect(payload.variables?.SLACK_ALLOWED_CHANNEL_IDS?.value ?? "").toBe("");
  expect(payload.variables?.SLACK_ALLOWED_USER_IDS?.value ?? "").toBe("");
});

// Core overlap regression: one key targeted to both agent + interface must still be treated
// as filled and included correctly when entered once in the UI.
test("overlapping slack bot token targets still deploy when token is filled in UI", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_OVERLAP}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /slack/i }).click();

  await expect(page.getByLabel("Slack Bot Token")).toBeVisible();
  await expect(page.getByLabel("Slack App Token")).toBeVisible();

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByLabel("Slack Bot Token").fill("xoxb-overlap-value");
  await page.getByLabel("Slack App Token").fill("xapp-overlap-value");

  const deployRequest = page.waitForRequest((request) =>
    request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.waitForURL("**/agents", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  const payload = (await deployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-overlap-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-overlap-value");
});

// Ensures bulk import writes values into both general variable fields and adapter credential
// fields, then deploy submission uses those imported values end-to-end.
test("import variables fills config and slack fields, then deploy uses imported values", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

  await page.getByRole("button", { name: /^import$/i }).click();

  const envContents = [
    "OPENAI_API_KEY=sk-imported-value",
    "SLACK_BOT_TOKEN=xoxb-imported-value",
    "SLACK_APP_TOKEN=xapp-imported-value",
    "SLACK_ACTIONABLE_REACTIONS=ticket, bug",
    "SLACK_ALLOWED_CHANNEL_IDS=C123, C999",
    "SLACK_ALLOWED_USER_IDS=U123, U999",
    "UNUSED_KEY=skip-me",
  ].join("\n");

  await page.locator('input[type="file"]').setInputFiles({
    name: ".env",
    mimeType: "text/plain",
    buffer: Buffer.from(envContents),
  });

  await expect(page.getByText(/Filled 6 variables/i)).toBeVisible();
  await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-imported-value");

  await page.getByRole("button", { name: /slack/i }).click();
  await expect(page.getByLabel("Slack Bot Token")).toHaveValue("xoxb-imported-value");
  await expect(page.getByLabel("Slack App Token")).toHaveValue("xapp-imported-value");
  await expect(page.getByLabel("Actionable Reactions")).toHaveValue("ticket, bug");
  await expect(page.getByLabel("Allowed Channel IDs")).toHaveValue("C123, C999");
  await expect(page.getByLabel("Allowed User IDs")).toHaveValue("U123, U999");

  const deployRequest = page.waitForRequest((request) =>
    request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.waitForURL("**/agents", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  const payload = (await deployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-imported-value");
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-imported-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-imported-value");
  expect(payload.variables?.SLACK_ACTIONABLE_REACTIONS?.value).toBe("ticket, bug");
  expect(payload.variables?.SLACK_ALLOWED_CHANNEL_IDS?.value).toBe("C123, C999");
  expect(payload.variables?.SLACK_ALLOWED_USER_IDS?.value).toBe("U123, U999");
});

// Configure-page redeploy coverage: edits to existing deployment credentials must flow into
// Save & Redeploy payload, not just initial install flow.
test("configure deployment save and redeploy sends updated slack bot token", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("Deployment not found")).toHaveCount(0);
  await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-existing-value");
  await expect(page.getByLabel("Slack Bot Token")).toHaveValue("xoxb-existing-value");
  await expect(page.getByLabel("Allowed Channel IDs")).toHaveValue("C123, C999");
  await expect(page.getByLabel("Allowed User IDs")).toHaveValue("U123, U999");

  await page.getByLabel("Slack Bot Token").fill("xoxb-redeployed-value");
  await page.getByLabel("Allowed Channel IDs").fill("C111, C222");
  await page.getByLabel("Allowed User IDs").fill("U111, U222");

  await expect(page.getByRole("button", { name: /save\s*&\s*redeploy/i })).toBeVisible();

  const redeployRequest = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    redeployRequest,
    page.waitForURL(`**/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}`, { timeout: 20_000 }),
    page.getByRole("button", { name: /save\s*&\s*redeploy/i }).click(),
  ]);

  const payload = (await redeployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-redeployed-value");
  expect(payload.variables?.SLACK_ALLOWED_CHANNEL_IDS?.value).toBe("C111, C222");
  expect(payload.variables?.SLACK_ALLOWED_USER_IDS?.value).toBe("U111, U222");
  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-existing-value");
});

// Configure-page overlap regression: overlapping token targets must remain populated across
// prefilled state and still serialize correctly after redeploy edits.
test("configure redeploy keeps overlapping slack token mapped and populated", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_OVERLAP_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("Deployment not found")).toHaveCount(0);
  await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-overlap-existing-value");
  await expect(page.getByLabel("Slack Bot Token")).toHaveValue("xoxb-overlap-existing-value");

  await page.getByLabel("Slack Bot Token").fill("xoxb-overlap-redeployed-value");

  const redeployRequest = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    redeployRequest,
    page.waitForURL(`**/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_OVERLAP_ID}`, { timeout: 20_000 }),
    page.getByRole("button", { name: /save\s*&\s*redeploy/i }).click(),
  ]);

  const payload = (await redeployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-overlap-redeployed-value");
  expect(payload.variables?.SLACK_APP_TOKEN?.value).toBe("xapp-overlap-existing-value");
});

// Validates server-error UX path: when backend rejects payload with validation_errors, user
// stays on form and sees actionable error text instead of silent failure or redirect.
test("shows server validation error when deploy API rejects slack bot token", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: /slack/i }).click();

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByLabel("Slack Bot Token").fill("xoxb-server-reject");
  await page.getByLabel("Slack App Token").fill("xapp-test-value");

  const deployRequest = page.waitForRequest((request) =>
    request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    deployRequest,
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  await expect(page).toHaveURL(new RegExp(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}$`));
  await expect(page.getByText("Deployment failed")).toBeVisible();
  await expect(
    page.getByText("variables.SLACK_BOT_TOKEN.value: required variable has no value"),
  ).toBeVisible();
});
