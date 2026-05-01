import { test, expect } from "@playwright/test";

test.describe("settings — variables & secrets", () => {
  test("profile avatar shows picture and can navigate to add variables", async ({ page }) => {
    await page.goto("/settings", { waitUntil: "load" });

    // Profile avatar — confirm picture is present
    const avatarBtn = page.getByRole("button", { name: /user menu for/i });
    await avatarBtn.waitFor({ state: "visible", timeout: 15000 });
    await expect(avatarBtn.locator("img")).toBeVisible();

    // Open user menu → Settings
    await avatarBtn.click();
    await page.getByRole("menuitem", { name: "Settings" }).click();
    await page.waitForURL(/\/settings/, { timeout: 10000 });

    // Navigate to Variables & Secrets via sidebar
    await page.getByRole("link", { name: /variables.*secrets|variables & secrets/i }).click();
    await page.waitForURL(/\/settings\/secrets/, { timeout: 10000 });
    await expect(page.getByRole("heading", { name: "Variables & Secrets" })).toBeVisible();

    // Open the New variable dialog
    await page.getByRole("button", { name: "New variable" }).first().click();
    await expect(page.getByRole("heading", { name: "New variable" })).toBeVisible();

    // First variable: ANTHROPIC_API_KEY — secret (toggle on by default)
    await page.getByRole("textbox", { name: "Key" }).nth(0).fill("ANTHROPIC_API_KEY");
    await page.getByRole("textbox", { name: "Value" }).nth(0).fill("sk_abc123");
    // Ensure secret toggle is on for the first variable
    const firstSecretToggle = page.getByRole("switch", { name: "Secret" }).nth(0);
    if (!(await firstSecretToggle.isChecked())) {
      await firstSecretToggle.click();
    }

    // Add a second row
    await page.getByRole("button", { name: "Add another" }).click();

    // Second variable: WEATHER_API_KEY — non-secret
    await page.getByRole("textbox", { name: "Key" }).nth(1).fill("WEATHER_API_KEY");
    await page.getByRole("textbox", { name: "Value" }).nth(1).fill("the-key-in-pt");
    // Turn secret off for the second variable
    const secondSecretToggle = page.getByRole("switch", { name: "Secret" }).nth(1);
    if (await secondSecretToggle.isChecked()) {
      await secondSecretToggle.click();
    }

    // Save
    await page.getByRole("button", { name: "Save" }).click();

    // Dialog should close and both variables appear in the list
    await expect(page.getByRole("heading", { name: "New variable" })).not.toBeVisible();

    // ANTHROPIC_API_KEY — secret: key name visible, value masked as dots, raw value never exposed
    await expect(page.getByText("ANTHROPIC_API_KEY")).toBeVisible();
    await expect(page.getByText("••••••••")).toBeVisible();
    await expect(page.getByText("sk_abc123")).not.toBeVisible();

    // WEATHER_API_KEY — non-secret: key name visible, plain text value shown
    await expect(page.getByText("WEATHER_API_KEY")).toBeVisible();
    await expect(page.getByText("the-key-in-pt")).toBeVisible();
  });
});

test.describe("weather-poet deploy", () => {
  test("deploy page auto-fills saved variables under Configuration", async ({ page }) => {
    await page.goto("/rabbah/weather-poet", { waitUntil: "load" });

    // Authenticated — clicking deploy goes directly to the deploy page, not login
    await page.getByRole("link", { name: /deploy this agent/i }).last().click();
    await page.waitForURL(/deploy\/rabbah\/weather-poet/, { timeout: 15000 });

    // Configuration section is present
    await expect(page.getByText("Configuration", { exact: true })).toBeVisible();

    // Both variable fields are visible with humanized labels
    await expect(page.getByText("Anthropic API Key", { exact: true })).toBeVisible();
    await expect(page.getByText("Weather API Key", { exact: true })).toBeVisible();

    // Both are auto-filled from the saved account variables
    await expect(page.getByText("Auto-filled").first()).toBeVisible();
    await expect(page.getByText("Auto-filled")).toHaveCount(2);
  });
});
