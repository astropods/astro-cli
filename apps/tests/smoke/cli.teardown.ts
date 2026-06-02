import { test, expect } from "./fixtures";
import { existsSync, readFileSync, rmSync } from "fs";
import { CLI_STATE_FILE, exec } from "./cli-state";

test("archive hello-astro and clean up sandbox", () => {
  if (!existsSync(CLI_STATE_FILE)) return;

  const { fakeHome, astBin, pushSucceeded } = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));

  if (pushSucceeded) {
    const archiveResult = exec(`${astBin} bp archive hello-astro`, {
      env: { ...process.env, HOME: fakeHome },
      encoding: "utf-8",
      timeout: 30000,
    });
    console.log(archiveResult);
    const cleanArchive = archiveResult.replace(/\x1b\[[0-9;]*m/g, "");
    expect(cleanArchive).toContain("Archiving blueprint hello-astro");
    expect(cleanArchive).toContain("✓ hello-astro archived");

    const deleteResult = exec(`${astBin} agent delete --name 'Hello Astro' --confirm 'Hello Astro'`, {
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
