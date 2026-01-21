import { Graph, z } from "astro-graph";

/**
 * Input schema for the GitHub URL parser workflow
 */
const ParseGithubUrlInputSchema = z.object({
  url: z.string(),
});

/**
 * Types of GitHub URLs that can be parsed
 */
type GithubUrlType =
  | "repository"
  | "user"
  | "issue"
  | "pull_request"
  | "file"
  | "unknown";

/**
 * Output type for the GitHub URL parser workflow
 */
type ParseGithubUrlOutput = {
  success: boolean;
  isValid: boolean;
  urlType: GithubUrlType;
  username?: string;
  repo?: string;
  branch?: string;
  path?: string;
  issueNumber?: number;
  prNumber?: number;
  summary: string;
  error?: string;
};

/**
 * Parsed URL intermediate type
 */
type ParsedUrl = {
  isValid: boolean;
  username?: string;
  pathParts: string[];
};

/**
 * A workflow that parses GitHub URLs and extracts useful information.
 * Takes a URL string and returns parsed components based on the URL type.
 * Uses branching logic to handle different URL types.
 */
export const parseGithubUrl = new Graph(ParseGithubUrlInputSchema)
  .meta({
    title: "Parse GitHub URL",
    description:
      "Parses a GitHub URL and extracts information like username, repo, and URL type (repository, issue, PR, file, etc.)",
  })
  // First, parse and validate the URL
  .run(
    (f) =>
      f.evaluate({
        fn: async (input): Promise<ParsedUrl> => {
          const { url } = input;
          const githubPattern =
            /^https?:\/\/(www\.)?github\.com\/([^/]+)(\/.*)?$/i;
          const match = url.match(githubPattern);

          if (!match) {
            return { isValid: false, pathParts: [] };
          }

          const username = match[2];
          const pathPart = match[3] || "";
          const pathParts = pathPart.split("/").filter(Boolean);

          return { isValid: true, username, pathParts };
        },
      }),
    { name: "Read the link" }
  )
  // Branch: Check if URL is valid
  .if(
    {
      condition: (input) => input.isValid,
      then: (branch) =>
        branch
          // Branch: Check if it's just a user profile (no path parts)
          .if(
            {
              condition: (input) => input.pathParts.length === 0,
              then: (userBranch) =>
                userBranch.run(
                  (f) =>
                    f.evaluate({
                      fn: async (input): Promise<ParseGithubUrlOutput> => ({
                        success: true,
                        isValid: true,
                        urlType: "user",
                        username: input.username,
                        summary: `GitHub user profile: ${input.username}`,
                      }),
                    }),
                  { name: "Found: User Profile" }
                ),
              else: (repoBranch) =>
                repoBranch
                  // Branch: Check if it's an issue URL
                  .if(
                    {
                      condition: (input) =>
                        input.pathParts.length >= 3 &&
                        input.pathParts[1] === "issues",
                      then: (issueBranch) =>
                        issueBranch.run(
                          (f) =>
                            f.evaluate({
                              fn: async (
                                input
                              ): Promise<ParseGithubUrlOutput> => {
                                const issueNumber = parseInt(
                                  input.pathParts[2],
                                  10
                                );
                                return {
                                  success: true,
                                  isValid: true,
                                  urlType: "issue",
                                  username: input.username,
                                  repo: input.pathParts[0],
                                  issueNumber: isNaN(issueNumber)
                                    ? undefined
                                    : issueNumber,
                                  summary: `GitHub issue #${issueNumber} in ${input.username}/${input.pathParts[0]}`,
                                };
                              },
                            }),
                          { name: "Found: Issue" }
                        ),
                      else: (nonIssueBranch) =>
                        nonIssueBranch
                          // Branch: Check if it's a PR URL
                          .if(
                            {
                              condition: (input) =>
                                input.pathParts.length >= 3 &&
                                input.pathParts[1] === "pull",
                              then: (prBranch) =>
                                prBranch.run(
                                  (f) =>
                                    f.evaluate({
                                      fn: async (
                                        input
                                      ): Promise<ParseGithubUrlOutput> => {
                                        const prNumber = parseInt(
                                          input.pathParts[2],
                                          10
                                        );
                                        return {
                                          success: true,
                                          isValid: true,
                                          urlType: "pull_request",
                                          username: input.username,
                                          repo: input.pathParts[0],
                                          prNumber: isNaN(prNumber)
                                            ? undefined
                                            : prNumber,
                                          summary: `GitHub pull request #${prNumber} in ${input.username}/${input.pathParts[0]}`,
                                        };
                                      },
                                    }),
                                  { name: "Found: Pull Request" }
                                ),
                              else: (nonPrBranch) =>
                                nonPrBranch
                                  // Branch: Check if it's a file URL (blob)
                                  .if(
                                    {
                                      condition: (input) =>
                                        input.pathParts.length >= 3 &&
                                        input.pathParts[1] === "blob",
                                      then: (fileBranch) =>
                                        fileBranch.run(
                                          (f) =>
                                            f.evaluate({
                                              fn: async (
                                                input
                                              ): Promise<ParseGithubUrlOutput> => {
                                                const branch =
                                                  input.pathParts[2];
                                                const filePath = input.pathParts
                                                  .slice(3)
                                                  .join("/");
                                                return {
                                                  success: true,
                                                  isValid: true,
                                                  urlType: "file",
                                                  username: input.username,
                                                  repo: input.pathParts[0],
                                                  branch,
                                                  path: filePath || undefined,
                                                  summary: `GitHub file${filePath ? `: ${filePath}` : ""} on branch ${branch} in ${input.username}/${input.pathParts[0]}`,
                                                };
                                              },
                                            }),
                                          { name: "Found: File" }
                                        ),
                                      else: (defaultBranch) =>
                                        // Default: treat as repository
                                        defaultBranch.run(
                                          (f) =>
                                            f.evaluate({
                                              fn: async (
                                                input
                                              ): Promise<ParseGithubUrlOutput> => ({
                                                success: true,
                                                isValid: true,
                                                urlType: "repository",
                                                username: input.username,
                                                repo: input.pathParts[0],
                                                summary: `GitHub repository: ${input.username}/${input.pathParts[0]}`,
                                              }),
                                            }),
                                          { name: "Found: Project" }
                                        ),
                                    },
                                    "Links to a file?"
                                  ),
                            },
                            "Links to a pull request?"
                          ),
                    },
                    "Links to an issue?"
                  ),
            },
            "Links to a project?"
          ),
      else: (invalidBranch) =>
        invalidBranch.run(
          (f) =>
            f.evaluate({
              fn: async (): Promise<ParseGithubUrlOutput> => ({
                success: true,
                isValid: false,
                urlType: "unknown",
                summary: "The provided URL is not a valid GitHub URL",
                error: "URL does not match GitHub URL pattern",
              }),
            }),
          { name: "Not a GitHub link" }
        ),
    },
    "Is this a GitHub link?"
  );
