import { Graph, z } from "@saswatds/astro-graph";

/**
 * Input schema for the GitHub README fetch workflow
 */
const FetchReadmeInputSchema = z.object({
  username: z.string(),
  repo: z.string(),
});

/**
 * Output type for the GitHub README fetch workflow
 */
type FetchReadmeOutput = {
  readme: string;
  success: boolean;
  error?: string;
};

/**
 * Helper to check if README content is just a path reference to another file
 */
function isPathReference(content: string): string | null {
  const trimmed = content.trim();
  // Check if content is just a path (e.g., "packages/ai/README.md")
  if (
    trimmed.match(/^[\w\-./]+\.md$/i) &&
    !trimmed.includes("\n") &&
    trimmed.length < 200
  ) {
    return trimmed;
  }
  return null;
}

/**
 * A workflow that fetches the README from a GitHub repository.
 * Takes a username and repo name, returns the README content.
 * If the root README is a path reference, follows it to get the actual content.
 */
export const fetchGithubReadme = new Graph(FetchReadmeInputSchema)
  .meta({
    title: "Fetch GitHub README",
    description:
      "Fetches the README.md content from a specified GitHub repository",
  })
  .run(
    (f) =>
      f.evaluate({
        fn: async (input): Promise<FetchReadmeOutput> => {
          const { username, repo } = input;

          // Try fetching from the main branch first, then master
          const branches = ["main", "master"];

          for (const branch of branches) {
            const url = `https://raw.githubusercontent.com/${username}/${repo}/${branch}/README.md`;

            try {
              const response = await fetch(url);

              if (response.ok) {
                let readme = await response.text();

                // Check if the README is just a path reference to another file
                const pathRef = isPathReference(readme);
                if (pathRef) {
                  // Fetch the actual README from the referenced path
                  const actualUrl = `https://raw.githubusercontent.com/${username}/${repo}/${branch}/${pathRef}`;
                  const actualResponse = await fetch(actualUrl);
                  if (actualResponse.ok) {
                    readme = await actualResponse.text();
                  }
                }

                return {
                  readme,
                  success: true,
                };
              }
            } catch {
              // Continue to next branch
            }
          }

          // If raw fetch failed, try the GitHub API
          try {
            const apiUrl = `https://api.github.com/repos/${username}/${repo}/readme`;
            const response = await fetch(apiUrl, {
              headers: {
                Accept: "application/vnd.github.v3.raw",
              },
            });

            if (response.ok) {
              let readme = await response.text();

              // Check if the README is just a path reference
              const pathRef = isPathReference(readme);
              if (pathRef) {
                // Try to fetch from the referenced path (try main first, then master)
                for (const branch of branches) {
                  const actualUrl = `https://raw.githubusercontent.com/${username}/${repo}/${branch}/${pathRef}`;
                  try {
                    const actualResponse = await fetch(actualUrl);
                    if (actualResponse.ok) {
                      readme = await actualResponse.text();
                      break;
                    }
                  } catch {
                    // Continue to next branch
                  }
                }
              }

              return {
                readme,
                success: true,
              };
            }

            return {
              readme: "",
              success: false,
              error: `Failed to fetch README: ${response.status} ${response.statusText}`,
            };
          } catch (error) {
            return {
              readme: "",
              success: false,
              error: `Error fetching README: ${
                error instanceof Error ? error.message : "Unknown error"
              }`,
            };
          }
        },
      }),
    { name: "Fetch README" }
  );
