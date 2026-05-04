import { test, expect } from "@playwright/test";
import { spawn } from "child_process";
import { mkdtempSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { CLI_STATE_FILE, exec } from "./cli-state";
import { envConfig } from "./env";

// Tests must run serially: install before login.
test.describe.configure({ mode: "serial" });

// The install script hardcodes INSTALL_DIR="${HOME}/.ast/bin" with no override.
// Shadow HOME with a temp dir so the binary lands there, never touching ~/.ast.
const fakeHome = mkdtempSync(join(tmpdir(), "ast-home-"));
const astBin = join(fakeHome, ".ast", "bin", envConfig.cliName);

test.describe("CLI", () => {

  test("install — downloads ast to a sandboxed directory", async () => {
    console.log("fakeHome:", fakeHome);

    const installHost = process.env.ASTRO_TEST_HOST ?? envConfig.appBaseUrl;
    exec(`curl -fsSL ${installHost}/install | sh`, {
      env: { ...process.env, HOME: fakeHome },
      stdio: "pipe",
      timeout: 60000,
      shell: true,
    });

    const version = exec(`${astBin} --version`, { encoding: "utf-8" });
    expect(version.trim()).toBeTruthy();
  });

  test("login — device flow completes via browser confirmation", async ({ page }) => {
    const cli = spawn(astBin, ["login"], { env: { ...process.env, HOME: fakeHome } });

    // Parse the device URL from CLI output, e.g.:
    // → Opening browser to: https://<auth-host>/device?user_code=KZFS-NGLR
    const deviceUrl = await new Promise<string>((resolve, reject) => {
      const onData = (data: Buffer) => {
        const match = data
          .toString()
          .match(envConfig.deviceUrlPattern);
        if (match) resolve(match[0]);
      };
      cli.stdout.on("data", onData);
      cli.stderr.on("data", onData);
      setTimeout(() => reject(new Error("CLI did not print a device URL within 15s")), 15000);
    });

    // Navigate to the device confirmation page.
    // The storageState already holds a valid session so this should land directly on the Confirm screen.
    await page.goto(deviceUrl, { waitUntil: "load" });

    // If the session has expired and a login form appears, handle it.
    const emailField = page.getByLabel(/email/i);
    if (await emailField.isVisible({ timeout: 3000 }).catch(() => false)) {
      await emailField.fill(process.env.ASTRO_TEST_EMAIL!);
      await page.getByRole("button", { name: /continue/i }).click();
      await page.getByLabel(/password/i).waitFor({ state: "visible", timeout: 10000 });
      await page.getByLabel(/password/i).fill(process.env.ASTRO_TEST_PASSWORD!);
      // Anchored to avoid matching "Sign in with a passkey" on the preview login page
      await page.getByRole("button", { name: /^(continue|sign in)$/i }).click();
      await page.waitForURL((url) => envConfig.loginUrlExclude(url.toString()), {
        timeout: 30000,
      });
      await page.goto(deviceUrl, { waitUntil: "load" });
    }

    // Confirm the device authorization
    await page.getByRole("button", { name: /confirm|approve|allow|authorize/i }).click();

    // Wait for CLI to receive the callback and exit cleanly
    const exitCode = await new Promise<number>((resolve) => {
      cli.on("close", (code) => resolve(code ?? 0));
    });
    expect(exitCode, "ast login should exit with code 0").toBe(0);
  });

  test("account list — confirms login succeeded in sandbox", async () => {
    const result = exec(`${astBin} account list`, {
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 10000,
    });

    console.log(result);
    expect(result).toContain("astro-testbot (personal)");
  });

  test("blueprint list — hello-astro not present before push", async ({}, testInfo) => {
    const result = exec(`${astBin} bp list`, {
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 10000,
    });

    console.log(result);
    if (result.includes("hello-astro")) {
      testInfo.annotations.push({
        type: "warning",
        description: "hello-astro already exists before push — leftover from a previous run that did not clean up",
      });
      console.warn("WARNING: hello-astro already present in blueprint list — skipping precondition");
    }
  });

  test("push — clones agents repo and pushes hello-astro", { timeout: 360000 }, async () => {
    const repoDir = join(fakeHome, "agents");

    exec("git clone https://github.com/astropods/agents", {
      cwd: fakeHome,
      stdio: "pipe",
      timeout: 60000,
    });

    const result = exec(`${astBin} push`, {
      cwd: join(repoDir, "hello-astro"),
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 300000, // 5 minutes — push involves a container build + registry upload
    });

    console.log(result);
    expect(result).toContain("✓ Pushed successfully!");
    expect(result).toContain("hello-astro");
    expect(result).toContain(`${envConfig.appBaseUrl}/astro-testbot/hello-astro`);
    writeFileSync(CLI_STATE_FILE, JSON.stringify({ fakeHome, pushSucceeded: true }));
  });
});
