import { workflows } from "astro-workflows";
import { AstroAgent } from "../agent";

export const exampleGithubAgent = new AstroAgent()
  .meta({
    title: "Example GitHub Agent",
    description:
      "An example agent that can fetch readmes from GitHub repositories and parse GitHub URLs",
  })
  .instructions(
    "You are a helpful assistant that can fetch readmes from GitHub repositories and answer questions about them for the user. You can also parse GitHub URLs to extract information like the username, repository name, issue numbers, PR numbers, and more."
  )
  .tool({
    type: "graph",
    graph: workflows.fetchGithubReadme.compile(),
  })
  .tool({
    type: "graph",
    graph: workflows.parseGithubUrl.compile(),
  });
