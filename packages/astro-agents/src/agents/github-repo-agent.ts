import { Graph } from "astro-graph";

export const githubRepoAgent = new Graph<{
  repo: string;
  username: string;
  question: string;
}>()
  .meta({
    title: "GitHub Repo Agent",
    description: "Use AI to answer questions about a GitHub repo",
  })
  // Step 1: Fetch the README from GitHub
  .run(
    (f) =>
      f.evaluate({
        fn: async (input) => {
          // Try main branch first, then fallback to master
          const branches = ["main", "master"];

          for (const branch of branches) {
            const response = await fetch(
              `https://raw.githubusercontent.com/${input.username}/${input.repo}/${branch}/README.md`
            );

            if (response.ok) {
              const readme = await response.text();
              return {
                readme,
                question: input.question,
              };
            }
          }

          throw new Error(
            `Failed to fetch README for ${input.username}/${input.repo}`
          );
        },
      }),
    { name: "Fetch README from GitHub" }
  )
  // Step 2: Generate an answer using the README context
  .run(
    (f) =>
      f.generateText({
        model: "openai:gpt-4",
      }),
    {
      name: "Answer Question with AI",
      transform: (input) => ({
        prompt: input.question,
        system: `You are a helpful assistant that answers questions about a GitHub repository based on its README. Answer the user's question based on the following README content:\n\n${input.readme}`,
      }),
    }
  );
