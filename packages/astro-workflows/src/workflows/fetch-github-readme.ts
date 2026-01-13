import { Graph } from "astro-graph";

/**
 * Input type for the GitHub README fetch workflow
 */
type FetchReadmeInput = {
  username: string;
  repo: string;
};

/**
 * Output type for the GitHub README fetch workflow
 */
type FetchReadmeOutput = {
  readme: string;
  success: boolean;
  error?: string;
};

/**
 * A workflow that fetches the README from a GitHub repository.
 * Takes a username and repo name, returns the README content.
 */
export const fetchGithubReadme = new Graph<FetchReadmeInput>()
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
                const readme = await response.text();
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
              const readme = await response.text();
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
