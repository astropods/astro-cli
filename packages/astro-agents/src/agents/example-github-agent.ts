import { workflows } from "astro-workflows";
import { AstroAgent } from "../agent";

export const exampleGithubAgent = new AstroAgent()
  .meta({
    title: "Example GitHub Agent",
    description:
      "An example agent that can fetch readmes from GitHub repositories",
  })
  .instructions(
    "You are a helpful assistant that can fetch readmes from GitHub repositories and answer questions about them for the user."
  )
  .tool({
    type: "graph",
    graph: workflows.fetchGithubReadme.compile(),
  });
