import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { clickMenuItem, resetMockBackend } from "./helpers";

const DEPLOYMENT_ID = "dep-slack-full-1";
const RED_PIXEL_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==",
  "base64",
);

test.beforeEach(async ({ request }) => {
  await resetMockBackend(request);
});

async function sampleDownloadedPngAvatarCenter(page: Page, pngDataUri: string) {
  return page.evaluate(async (src) => {
    const image = new Image();
    image.src = src;
    await image.decode();

    const canvas = document.createElement("canvas");
    canvas.width = image.width;
    canvas.height = image.height;

    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("2D canvas context unavailable");

    ctx.drawImage(image, 0, 0);
    const x = Math.round(image.width * 0.5);
    const y = Math.round(image.height * (122 / 560));
    const [r, g, b, a] = ctx.getImageData(x, y, 1, 1).data;
    return { r, g, b, a, width: image.width, height: image.height };
  }, pngDataUri);
}

test("downloaded agent badge PNG includes the avatar image", async ({ page }) => {
  test.setTimeout(45_000);

  await page.route(`**/assets/avatars/deployments/${DEPLOYMENT_ID}.jpg`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "image/png",
      body: RED_PIXEL_PNG,
    });
  });

  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const card = page.locator(`[data-deployment-id="${DEPLOYMENT_ID}"]`);
  await expect(card.getByText("Slack Full Bot")).toBeVisible({ timeout: 10_000 });

  await clickMenuItem(
    page,
    async () => {
      await card.hover();
      await card.getByRole("button", { name: "Agent options" }).click();
    },
    "Share agent badge",
  );

  const shareButton = page.getByRole("button", { name: "Share badge" });
  await expect(shareButton).toBeVisible({ timeout: 10_000 });
  await shareButton.click();

  const downloadPngItem = page.getByRole("menuitem", { name: "Download PNG" });
  await expect(downloadPngItem).toBeVisible();

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    downloadPngItem.click({ force: true }),
  ]);

  const downloadPath = await download.path();
  expect(downloadPath).toBeTruthy();

  const png = await readFile(downloadPath!);
  const avatarPixel = await sampleDownloadedPngAvatarCenter(
    page,
    `data:image/png;base64,${png.toString("base64")}`,
  );

  expect(avatarPixel.width).toBe(700);
  expect(avatarPixel.height).toBe(1120);
  expect(avatarPixel.r).toBeGreaterThan(240);
  expect(avatarPixel.g).toBeLessThan(20);
  expect(avatarPixel.b).toBeLessThan(20);
  expect(avatarPixel.a).toBe(255);
});
