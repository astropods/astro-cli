import { Graph } from "astro-graph";

type NpmPackageData = {
  name: string;
  version: string;
  description: string;
  readme: string;
  license: string;
  homepage: string;
  repository: { url: string } | string;
  dependencies: Record<string, string>;
  devDependencies: Record<string, string>;
  maintainers: Array<{ name: string; email: string }>;
  time: { created: string; modified: string };
  keywords: string[];
};

type PackageAnalysis = {
  packageData: NpmPackageData;
  question: string;
  isPopular: boolean;
  weeklyDownloads: number;
};

export const npmPackageAgent = new Graph<{
  packageName: string;
  question: string;
}>()
  .meta({
    title: "NPM Package Analyzer",
    description:
      "Analyze npm packages for quality, dependencies, and usage recommendations",
  })
  // Step 1: Fetch package metadata from npm registry
  .run(
    (f) =>
      f.evaluate({
        fn: async (input) => {
          const [registryRes, downloadsRes] = await Promise.all([
            fetch(`https://registry.npmjs.org/${input.packageName}`),
            fetch(
              `https://api.npmjs.org/downloads/point/last-week/${input.packageName}`
            ),
          ]);

          if (!registryRes.ok) {
            throw new Error(`Package "${input.packageName}" not found on npm`);
          }

          const packageData: NpmPackageData = await registryRes.json();
          const downloadsData = downloadsRes.ok
            ? await downloadsRes.json()
            : { downloads: 0 };

          return {
            packageData,
            question: input.question,
            weeklyDownloads: downloadsData.downloads as number,
            isPopular: downloadsData.downloads > 100000,
          };
        },
      }),
    { name: "Fetch Package from NPM Registry" }
  )
  // Step 2: Branch based on package popularity
  .if(
    {
      condition: (input) => input.isPopular,
      // Popular packages get detailed analysis
      then: (branch) =>
        branch.run(
          (f) =>
            f.generateText({
              model: "openai:gpt-4",
            }),
          {
            name: "Detailed Analysis (Popular Package)",
            transform: (input: PackageAnalysis) => {
              const deps = Object.keys(input.packageData.dependencies || {});
              const devDeps = Object.keys(
                input.packageData.devDependencies || {}
              );

              return {
                prompt: input.question,
                system: `You are an expert npm package analyst providing DETAILED analysis for popular, well-established packages.

## Package: ${input.packageData.name}@${input.packageData.version}

### Overview
- **Description:** ${input.packageData.description || "No description"}
- **License:** ${input.packageData.license || "Unknown"}
- **Weekly Downloads:** ${input.weeklyDownloads.toLocaleString()} (Popular!)
- **Keywords:** ${input.packageData.keywords?.join(", ") || "None"}

### Maintenance
- **Created:** ${input.packageData.time?.created || "Unknown"}
- **Last Modified:** ${input.packageData.time?.modified || "Unknown"}
- **Maintainers:** ${
                  input.packageData.maintainers
                    ?.map((m) => m.name)
                    .join(", ") || "Unknown"
                }

### Dependencies (${deps.length} production, ${devDeps.length} dev)
${
  deps.length > 0
    ? deps.slice(0, 15).join(", ") + (deps.length > 15 ? "..." : "")
    : "None"
}

### README Preview
${input.packageData.readme?.slice(0, 2000) || "No README available"}

---
Provide a thorough, detailed analysis. This is a well-established package, so focus on best practices, advanced usage patterns, and potential gotchas.`,
              };
            },
          }
        ),
      // Less popular packages get a simpler overview
      else: (branch) =>
        branch.run(
          (f) =>
            f.generateText({
              model: "openai:gpt-4",
            }),
          {
            name: "Quick Analysis (Less Popular Package)",
            transform: (input: PackageAnalysis) => {
              const deps = Object.keys(input.packageData.dependencies || {});

              return {
                prompt: input.question,
                system: `You are an npm package analyst providing a focused overview for less widely-used packages.

## Package: ${input.packageData.name}@${input.packageData.version}

- **Description:** ${input.packageData.description || "No description"}
- **License:** ${input.packageData.license || "Unknown"}
- **Weekly Downloads:** ${input.weeklyDownloads.toLocaleString()}
- **Dependencies:** ${deps.length} (${deps.slice(0, 5).join(", ")}${
                  deps.length > 5 ? "..." : ""
                })
- **Last Modified:** ${input.packageData.time?.modified || "Unknown"}

### README Preview
${input.packageData.readme?.slice(0, 1000) || "No README available"}

---
Since this is a less popular package, also consider:
- Is it actively maintained?
- Are there more popular alternatives?
- Any red flags to watch out for?`,
              };
            },
          }
        ),
    },
    "If Weekly Downloads > 100k"
  );
