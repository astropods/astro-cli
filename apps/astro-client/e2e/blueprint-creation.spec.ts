import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("shows min-length hint and disables continue button when name is too short", async ({ page }) => {
  test.setTimeout(20_000);
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("my-agent").fill("ab");

  await expect(page.getByText(/at least 4 characters/i)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeDisabled();
});

test("shows letter-start hint and disables continue button when name begins with a digit", async ({ page }) => {
  test.setTimeout(20_000);
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("my-agent").fill("1abc");

  await expect(page.getByText(/must start with a letter/i)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeDisabled();
});

test("shows 'already exists' error and disables continue button for an existing blueprint name", async ({ page }) => {
  test.setTimeout(20_000);
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  // "code-reviewer" is a pre-existing agent in the mock — name check returns 200
  await page.getByPlaceholder("my-agent").fill("code-reviewer");

  await expect(page.getByText(/already exists/i)).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(`${ACCOUNT}/code-reviewer`)).toBeVisible();
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeDisabled();
});

test("shows 'will be created as' hint and enables continue button for a valid available name", async ({ page }) => {
  test.setTimeout(20_000);
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  // "mynewagent" is unknown to the mock — name check returns 404 (not taken)
  await page.getByPlaceholder("my-agent").fill("mynewagent");

  await expect(page.getByText(/will be created as/i)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(`${ACCOUNT}/mynewagent`)).toBeVisible();
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled();
});

test("happy path: creates blueprint, shows publishing panel, shows review panel, then navigates to detail", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled({ timeout: 5_000 });
  await page.getByRole("button", { name: /^continue$/i }).click();

  // Source step — select "Set up locally"
  await expect(page.getByText(/starting point/i)).toBeVisible({ timeout: 5_000 });
  await page.getByText(/set up locally/i).click();

  // Intercept the create blueprint request and assert payload
  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );

  await page.getByRole("button", { name: /create blueprint/i }).click();

  const req = await createReq;
  const payload = req.postDataJSON() as { name: string; visibility?: string };
  expect(payload.name).toBe("mynewagent");
  expect(payload.visibility).toBe("private"); // default selection

  // Publishing panel is shown during the initialization delay
  await expect(page.getByText(/initializing mynewagent/i)).toBeVisible({ timeout: 10_000 });

  // Once the review panel polls the detail endpoint, it finds versions and auto-navigates
  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 10_000 });
});
