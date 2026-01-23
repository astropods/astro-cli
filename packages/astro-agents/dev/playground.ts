#!/usr/bin/env bun
/**
 * Development script to run the playground with agents.
 * Run with: bun run playground (or: bun dev/playground.ts)
 *
 * Add new agents by importing them below and adding to the agents object.
 */

import { startPlayground } from "astro-playground";

import { githubAgent } from "../github-agent/src/index";

// Start the playground with all agents
await startPlayground({
  agents: {
    githubAgent,
  },
});
