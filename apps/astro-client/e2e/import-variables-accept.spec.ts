import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT = "slack-config-full";

test.beforeEach(async ({ request }) => {
  await resetMockBackend(request);
});

test.describe("import variables file picker", () => {
  test("accept hint lists env/json/txt including compound names, and imports .env.local", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("button", { name: /^import$/i })).toBeVisible({ timeout: 20_000 });
    await page.getByRole("button", { name: /^import$/i }).click();

    const fileInput = page.locator('input[type="file"]');
    await fileInput.waitFor({ state: "attached", timeout: 10_000 });

    // The accept hint must cover base and compound forms so the OS picker does not
    // grey out valid files like .env.local (browsers match accept by exact suffix).
    const accept = (await fileInput.getAttribute("accept")) ?? "";
    const tokens = accept.split(",");
    for (const expected of [".env", ".env.local", ".json", ".txt"]) {
      expect(tokens).toContain(expected);
    }
    // Broad MIME tokens are intentionally excluded so the hint does not keep
    // clearly-invalid files (e.g. .md, .csv reported as text/plain) selectable.
    for (const notExpected of ["text/plain", "application/json"]) {
      expect(tokens).not.toContain(notExpected);
    }

    // A compound-named dotenv file flows through to the variable fields.
    await fileInput.setInputFiles({
      name: ".env.local",
      mimeType: "text/plain",
      buffer: Buffer.from("OPENAI_API_KEY=sk-compound-value"),
    });

    await expect(page.getByLabel("Openai Api Key")).toHaveValue("sk-compound-value");
  });
});
