import { expect, test } from "@playwright/test";
import { expectEventually, resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";

const TINY_PNG = Buffer.from(
  "89504e470d0a1a0a0000000d494844520000000100000001080200000090775" +
    "3de0000000c4944415408d763f8cfc000000002000" +
    "1e221bc330000000049454e44ae426082",
  "hex",
);

test.beforeEach(async () => {
  await resetMockBackend();
});

test("profile avatar upload: edit sidebar uploads cropped image and closes dialog", async ({ page }) => {
  await page.goto(`/${ACCOUNT}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: /edit profile/i })).toBeVisible();

  await page.getByRole("button", { name: /edit profile/i }).click();
  await expect(page.getByRole("heading", { name: /edit profile/i })).toBeVisible();

  await page.getByRole("button", { name: ACCOUNT, exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Upload profile image")).toBeVisible({ timeout: 5_000 });

  await page.locator('input[type="file"]').setInputFiles({
    name: "avatar.png",
    mimeType: "image/png",
    buffer: TINY_PNG,
  });

  await expectEventually(async () => {
    await expect(page.getByText(/adjust the crop/i)).toBeVisible();
  });

  const uploadReq = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().includes(`/accounts/${ACCOUNT}/avatar`),
  );

  await page.getByRole("button", { name: "Upload" }).click();
  await uploadReq;

  await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10_000 });
});
