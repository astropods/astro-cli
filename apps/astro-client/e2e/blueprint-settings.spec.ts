import { expect, test } from "@playwright/test";
import path from "path";

const MOCK_BACKEND = "http://localhost:48787";
const ACCOUNT = "testuser";

// Minimal valid 1×1 PNG (binary)
const TINY_PNG = Buffer.from(
  "89504e470d0a1a0a0000000d494844520000000100000001080200000090775" +
  "3de0000000c4944415408d763f8cfc000000002000" +
  "1e221bc330000000049454e44ae426082",
  "hex",
);

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("avatar upload on blueprint detail: dialog opens, file uploads, POST fires", async ({ page }) => {
  test.setTimeout(30_000);

  // code-reviewer is owned by testuser → canEdit=true → camera button is shown
  await page.goto(`/${ACCOUNT}/code-reviewer`, { waitUntil: "domcontentloaded" });

  // Wait for the page header to render (blueprint name visible)
  await expect(page.getByRole("heading", { name: "code-reviewer" })).toBeVisible({ timeout: 15_000 });

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

  // Cropper phase: "Adjust the crop, then upload."
  await expect(page.getByText(/adjust the crop/i)).toBeVisible({ timeout: 5_000 });

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
  test.setTimeout(30_000);

  // Account profile lists blueprints with archive button for the owner
  await page.goto(`/${ACCOUNT}`, { waitUntil: "domcontentloaded" });

  // Wait for blueprint cards to render (name appears in both card title and description — use first())
  await expect(page.getByText("code-reviewer").first()).toBeVisible({ timeout: 15_000 });

  // Open the options dropdown for code-reviewer card
  const blueprintCard = page.locator("[aria-label='Blueprint options']").first();
  await expect(blueprintCard).toBeVisible({ timeout: 5_000 });
  await blueprintCard.click();

  // Click "Archive agent" in the dropdown
  await page.getByRole("menuitem", { name: /archive agent/i }).click();

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
