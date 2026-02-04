import { workflows } from "@saswatds/astro-workflows";
import { AstroAgent } from "@saswatds/astro-agent";

/**
 * GitHub Agent Example
 *
 * Demonstrates how to build an agent that can fetch and analyze GitHub repositories.
 * Uses astro-workflows for GitHub integration.
 */

export const githubAgent = new AstroAgent()
  .meta({
    title: "GitHub Assistant",
    description: "An agent that can fetch readmes from GitHub repositories and answer questions about them",
  })
  .instructions(
    "You are a helpful assistant that can fetch readmes from GitHub repositories and answer questions about them for the user."
  )
  .tool({
    type: "graph",
    graph: workflows.fetchGithubReadme.compile(),
  });
