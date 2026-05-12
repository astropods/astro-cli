import { test, expect } from "./fixtures";
import { existsSync, readFileSync } from "fs";
import { CLI_STATE_FILE, exec } from "./cli-state";

test("ast agent list — deployment slug is present after deploy", () => {
  if (!existsSync(CLI_STATE_FILE)) {
    throw new Error("CLI state file not found");
  }

  const { fakeHome, astBin, deploymentSlug } = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));

  const result = exec(`${astBin} agent list`, {
    env: { ...process.env, HOME: fakeHome },
    encoding: "utf-8",
    timeout: 15000,
  });

  console.log(result);
  console.log("checking for deploymentSlug:", deploymentSlug);
  expect(result).toContain("hello-astro");
  expect(result).toContain(deploymentSlug);
});
