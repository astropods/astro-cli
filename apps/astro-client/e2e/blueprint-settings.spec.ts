import { expect, test } from "@playwright/test";
import { expectEventually, resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";

// Minimal valid 1×1 PNG (binary)
const TINY_PNG = Buffer.from(
  "89504e470d0a1a0a0000000d494844520000000100000001080200000090775" +
  "3de0000000c4944415408d763f8cfc000000002000" +
  "1e221bc330000000049454e44ae426082",
  "hex",
);

test.beforeEach(async () => {
  await resetMockBackend();
});

test("avatar upload on blueprint detail: dialog opens, file uploads, POST fires", async ({ page }) => {
  await page.goto(`/${ACCOUNT}/code-reviewer`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "code-reviewer" })).toBeVisible();

  // Click the avatar camera button to open the upload dialog
  const avatarButton = page.locator("button.group").filter({ has: page.locator("svg") }).first();
  await avatarButton.click();

  // Dialog: "Upload blueprint image"
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Upload blueprint image")).toBeVisible({ timeout: 5_000 });

  // Upload the tiny PNG via the hidden file input
  await page.locator('input[type="file"]').setInputFiles({
    name: "avatar.png",
    mimeType: "image/png",
    buffer: TINY_PNG,
  });

  // Cropper phase can be slow on CI while the image decodes.
  await expectEventually(async () => {
    await expect(page.getByText(/adjust the crop/i)).toBeVisible();
  });

  // Capture the avatar upload request
  const uploadReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/agents/${ACCOUNT}/code-reviewer/avatar`),
  );

  await page.getByRole("button", { name: "Upload" }).click();
  await uploadReq;

  // Dialog closes on success
  await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10_000 });
});

test("archive blueprint from account profile: dialog, confirm, POST fires", async ({ page }) => {
  await page.goto(`/${ACCOUNT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByText("code-reviewer").first()).toBeVisible();

  // Open the options dropdown specifically for the code-reviewer card.
  // Sort by "newest" puts blueprints with more versions first, so we cannot
  // rely on .first() across all [aria-label='Blueprint options'] buttons.
  const codeReviewerCard = page.locator(`a[href$="/${ACCOUNT}/code-reviewer"]`);
  const blueprintOptionsBtn = codeReviewerCard.locator("[aria-label='Blueprint options']");
  await expect(blueprintOptionsBtn).toBeVisible({ timeout: 5_000 });
  await blueprintOptionsBtn.click();

  // Click "Archive" in the dropdown
  await page.getByRole("menuitem", { name: /^archive$/i }).click();

  // Archive dialog appears
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText(/archive code-reviewer/i)).toBeVisible({ timeout: 5_000 });

  // Check the confirmation checkbox
  await page.getByRole("checkbox").check();

  // Type the blueprint name to confirm
  await page.getByPlaceholder("code-reviewer").fill("code-reviewer");

  // "Archive blueprint" button should now be enabled
  await expect(page.getByRole("button", { name: /archive blueprint/i })).toBeEnabled({ timeout: 5_000 });

  const archiveReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/agents/${ACCOUNT}/code-reviewer/archive`),
  );
  await page.getByRole("button", { name: /archive blueprint/i }).click();
  await archiveReq;
});
