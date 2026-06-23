import { expect, test } from "@playwright/test";
import { expectEventually, resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("blocks continue on submit with a min-length error when the name is too short", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("ab");
  await expect(page.getByText(/at least 4 characters/i)).toBeVisible();

  // The gate runs on submit: the button stays enabled but refuses to advance.
  const continueBtn = page.getByRole("button", { name: /^continue$/i });
  await expect(continueBtn).toBeEnabled();
  await continueBtn.click();

  await expect(page.getByText(/at least 4 characters/i)).toBeVisible();
  await expect(page.getByText(/starting point/i)).not.toBeVisible();
});

test("blocks continue on submit with a letter-start error when the name begins with a digit", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("1abc");
  await expect(page.getByText(/must start with a letter/i)).toBeVisible();

  const continueBtn = page.getByRole("button", { name: /^continue$/i });
  await expect(continueBtn).toBeEnabled();
  await continueBtn.click();

  await expect(page.getByText(/must start with a letter/i)).toBeVisible();
  await expect(page.getByText(/starting point/i)).not.toBeVisible();
});

test("blocks continue on submit for an existing blueprint name", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  // "code-reviewer" is pre-existing — availability check is debounced.
  await page.getByPlaceholder("my-agent").fill("code-reviewer");

  await expectEventually(async () => {
    await expect(page.getByText(/already exists/i)).toBeVisible();
    await expect(page.getByText(`${ACCOUNT}/code-reviewer`)).toBeVisible();
  });

  const continueBtn = page.getByRole("button", { name: /^continue$/i });
  await expect(continueBtn).toBeEnabled();
  await continueBtn.click();

  await expect(page.getByText(/already exists/i)).toBeVisible();
  await expect(page.getByText(/starting point/i)).not.toBeVisible();
});

test("shows 'will be created as' hint and enables continue button for a valid available name", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("mynewagent");

  await expectEventually(async () => {
    await expect(page.getByText(/will be created as/i)).toBeVisible();
    await expect(page.getByText(`${ACCOUNT}/mynewagent`)).toBeVisible();
    await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  });
});

test("happy path: creates blueprint, shows publishing panel, shows review panel, then navigates to detail", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });
  await expect(page.getByPlaceholder("my-agent")).toBeVisible();

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await expectEventually(async () => {
    await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  });
  await page.getByRole("button", { name: /^continue$/i }).click();

  await expect(page.getByText(/starting point/i)).toBeVisible();
  await page.getByText(/set up locally/i).click();

  const createReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/api/v1/agents/${ACCOUNT}`),
  );

  await page.getByRole("button", { name: /create blueprint/i }).click();

  const req = await createReq;
  const payload = req.postDataJSON() as { name: string; visibility?: string };
  expect(payload.name).toBe("mynewagent");
  expect(payload.visibility).toBe("private");

  await expect(page.getByText(/initializing mynewagent/i)).toBeVisible();

  await page.waitForURL(`**/${ACCOUNT}/mynewagent`, { timeout: 30_000 });
});
