import { test, expect } from "@playwright/test";
import { execSync } from "child_process";
import { existsSync, readFileSync, rmSync } from "fs";
import { join } from "path";
import { CLI_STATE_FILE } from "./cli-state";
import { envConfig } from "./env";

test("archive hello-astro and clean up sandbox", () => {
  if (!existsSync(CLI_STATE_FILE)) return;

  const { fakeHome, pushSucceeded } = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));
  const astBin = join(fakeHome, ".ast", "bin", envConfig.cliName);

  if (pushSucceeded) {
    const archiveResult = execSync(`${astBin} bp archive hello-astro`, {
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 30000,
    });
    console.log(archiveResult);
    const cleanArchive = archiveResult.replace(/\x1b\[[0-9;]*m/g, "");
    expect(cleanArchive).toContain("Archiving blueprint hello-astro");
    expect(cleanArchive).toContain("✓ hello-astro archived");

    const deleteResult = execSync(`${astBin} agent delete 'Hello Astro' --confirm 'Hello Astro'`, {
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 30000,
    });
    console.log(deleteResult);
  }

  rmSync(CLI_STATE_FILE, { force: true });

  if (!process.env.DEBUG_CLI_TESTS) {
    rmSync(fakeHome, { recursive: true, force: true });
  }
});
