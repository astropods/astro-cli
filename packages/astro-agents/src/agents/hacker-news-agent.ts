import { Graph } from "astro-graph";

type HNStory = {
  id: number;
  title: string;
  url?: string;
  score: number;
  by: string;
  time: number;
  descendants: number; // comment count
  type: "story" | "job" | "poll";
};

type HNAnalysis = {
  stories: HNStory[];
  question: string;
  category: "top" | "new" | "best" | "ask" | "show";
  storyCount: number;
};

const CATEGORY_ENDPOINTS: Record<string, string> = {
  top: "https://hacker-news.firebaseio.com/v0/topstories.json",
  new: "https://hacker-news.firebaseio.com/v0/newstories.json",
  best: "https://hacker-news.firebaseio.com/v0/beststories.json",
  ask: "https://hacker-news.firebaseio.com/v0/askstories.json",
  show: "https://hacker-news.firebaseio.com/v0/showstories.json",
};

export const hackerNewsAgent = new Graph<{
  question: string;
  category?: "top" | "new" | "best" | "ask" | "show";
  storyCount?: number;
}>()
  .meta({
    title: "Hacker News Agent",
    description:
      "Analyze trending stories on Hacker News and answer questions about current tech trends",
  })
  // Step 1: Fetch story IDs from the selected category
  .run(
    (f) =>
      f.evaluate({
        fn: async (input) => {
          const category = input.category || "top";
          const storyCount = Math.min(input.storyCount || 15, 30); // Cap at 30 to avoid too many requests

          const endpoint = CATEGORY_ENDPOINTS[category];
          const idsResponse = await fetch(endpoint);

          if (!idsResponse.ok) {
            throw new Error(
              `Failed to fetch ${category} stories from Hacker News`
            );
          }

          const storyIds: number[] = await idsResponse.json();
          const topIds = storyIds.slice(0, storyCount);

          // Fetch story details in parallel
          const storyPromises = topIds.map(async (id) => {
            const res = await fetch(
              `https://hacker-news.firebaseio.com/v0/item/${id}.json`
            );
            return res.ok ? res.json() : null;
          });

          const stories = (await Promise.all(storyPromises)).filter(
            (story): story is HNStory =>
              story !== null && story.type === "story"
          );

          return {
            stories,
            question: input.question,
            category,
            storyCount: stories.length,
          };
        },
      }),
    { name: "Fetch Stories from Hacker News" }
  )
  // Step 2: Branch based on whether we have enough high-engagement stories
  .if<string | object>(
    {
      condition: (input) => {
        // Check if we have stories with significant discussion (>50 comments avg)
        const avgComments =
          input.stories.reduce((sum, s) => sum + (s.descendants || 0), 0) /
          input.stories.length;
        return avgComments > 50;
      },
      // High engagement: provide detailed trend analysis
      then: (branch) =>
        branch.run(
          (f) =>
            f.generateText({
              model: "openai:gpt-4",
            }),
          {
            name: "Deep Trend Analysis (High Engagement)",
            transform: (input: HNAnalysis) => {
              const storySummaries = input.stories
                .map((s, i) => {
                  const domain = s.url
                    ? new URL(s.url).hostname.replace("www.", "")
                    : "self.hackernews";
                  const timeAgo = getTimeAgo(s.time);
                  return `${i + 1}. "${s.title}" [${s.score} pts, ${
                    s.descendants || 0
                  } comments, ${timeAgo}] (${domain}) by ${s.by}`;
                })
                .join("\n");

              const totalPoints = input.stories.reduce(
                (sum, s) => sum + s.score,
                0
              );
              const totalComments = input.stories.reduce(
                (sum, s) => sum + (s.descendants || 0),
                0
              );

              return {
                prompt: input.question,
                system: `You are an expert tech industry analyst with deep knowledge of Hacker News culture and the startup/tech ecosystem.

## Current ${input.category.toUpperCase()} Stories on Hacker News (${
                  input.storyCount
                } stories)

### Engagement Stats
- **Total Points:** ${totalPoints.toLocaleString()}
- **Total Comments:** ${totalComments.toLocaleString()}
- **Avg Points per Story:** ${Math.round(totalPoints / input.storyCount)}
- **Avg Comments per Story:** ${Math.round(totalComments / input.storyCount)}

### Stories
${storySummaries}

---

These stories have HIGH ENGAGEMENT, indicating the HN community finds them particularly noteworthy.

When answering:
- Identify emerging themes and patterns across stories
- Note any controversial topics (high comment-to-point ratios)
- Consider the HN audience perspective (developers, founders, tech enthusiasts)
- Reference specific stories by title when relevant
- If asked about trends, look for common threads across multiple stories`,
              };
            },
          }
        ),
      // Lower engagement: simpler overview
      else: (branch) =>
        branch.run(
          (f) =>
            f.generateText({
              model: "openai:gpt-4",
            }),
          {
            name: "Quick Overview (Standard Engagement)",
            transform: (input: HNAnalysis) => {
              const storySummaries = input.stories
                .map((s, i) => {
                  const domain = s.url
                    ? new URL(s.url).hostname.replace("www.", "")
                    : "self.hackernews";
                  return `${i + 1}. "${s.title}" [${s.score} pts, ${
                    s.descendants || 0
                  } comments] (${domain})`;
                })
                .join("\n");

              return {
                prompt: input.question,
                system: `You are a helpful assistant summarizing Hacker News stories.

## Current ${input.category.toUpperCase()} Stories on Hacker News

${storySummaries}

---

Provide a helpful, concise response to the user's question based on these stories.
If they're asking about trends, summarize what topics are currently popular.
Reference specific story titles when relevant.`,
              };
            },
          }
        ),
    },
    "High Engagement Check (Avg >50 comments)"
  );

// Helper function to format relative time
function getTimeAgo(unixTime: number): string {
  const seconds = Math.floor(Date.now() / 1000 - unixTime);

  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m ago`;
  } else if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    return `${hours}h ago`;
  } else {
    const days = Math.floor(seconds / 86400);
    return `${days}d ago`;
  }
}
