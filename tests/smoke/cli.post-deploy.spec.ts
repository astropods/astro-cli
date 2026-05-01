import { test, expect } from "@playwright/test";
import { execSync } from "child_process";
import { existsSync, readFileSync } from "fs";
import { join } from "path";
import { CLI_STATE_FILE } from "./cli-state";
import { envConfig } from "./env";

test("ast agent list — deployment slug is present after deploy", () => {
  if (!existsSync(CLI_STATE_FILE)) {
    throw new Error("CLI state file not found");
  }

  const { fakeHome, deploymentSlug } = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));
  const astBin = join(fakeHome, ".ast", "bin", envConfig.cliName);

  const result = execSync(`${astBin} agent list`, {
    env: { ...process.env, HOME: fakeHome },
    encoding: "utf-8",
    timeout: 15000,
  });

  console.log(result);
  console.log("checking for deploymentSlug:", deploymentSlug);
  expect(result).toContain("hello-astro");
  expect(result).toContain(deploymentSlug);
});
