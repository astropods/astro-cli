import type { Meta, StoryObj } from "@storybook/react-vite";
import { StyledMarkdown } from "@/components/StyledMarkdown";

const meta = {
  title: "Design System/Primitives/StyledMarkdown",
  component: StyledMarkdown,
  decorators: [
    (Story) => (
      <div className="max-w-3xl bg-surface p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StyledMarkdown>;

export default meta;
type Story = StoryObj<typeof meta>;

const KITCHEN_SINK = `# Heading 1

## Heading 2

### Heading 3

#### Heading 4

##### Heading 5

###### Heading 6

---

## Paragraphs and Inline Styles

This is a regular paragraph with **bold text**, *italic text*, and ***bold italic text***. You can also use ~~strikethrough~~ for deleted content.

Here is some \`inline code\` within a sentence. You can reference variables like \`const agent = new Agent()\` or file paths like \`astroai.yml\`.

This paragraph has a [link to Astro](https://astro.build) and an [autolinked URL](https://github.com). Links should be clearly distinguishable from surrounding text.

## Lists

### Unordered Lists

- First item in the list
- Second item with **bold text** and \`inline code\`
- Third item with a [link](https://example.com)
  - Nested item one
  - Nested item two
    - Deeply nested item
- Fourth item

### Ordered Lists

1. Clone the repository
2. Install dependencies with \`bun install\`
3. Configure your integrations in \`astroai.yml\`
4. Start the development server
   1. Run \`ast dev\` for local mode
   2. Run \`ast dev --remote\` for remote mode
5. Deploy when ready

### Task Lists

- [x] Set up project structure
- [x] Configure integrations
- [ ] Write agent logic
- [ ] Add tests
- [ ] Deploy to production

## Code Blocks

### JavaScript

\`\`\`javascript
import { Agent } from "@astro/sdk";

const agent = new Agent({
  name: "customer-insights-engine",
  version: "1.2.0",
});

agent.on("message", async (ctx) => {
  const insights = await ctx.analyze({
    sources: ["github", "slack", "linear"],
    timeframe: "7d",
  });

  return ctx.reply(insights.summary);
});

agent.start();
\`\`\`

### YAML Configuration

\`\`\`yaml
name: customer-insights-engine
version: 1.2.0

integrations:
  github:
    provider: github
    scopes: [read:repos, read:pulls]
  slack:
    provider: slack
    scopes: [channels:read, chat:write]

thresholds:
  alert_score: 0.75
  digest_frequency: daily

notify:
  channel: "#agent-alerts"
  method: slack
\`\`\`

### Bash

\`\`\`bash
# Install via Astro CLI
ast install customer-insights-engine

# Configure and start
ast config set --key GITHUB_TOKEN --value $TOKEN
ast dev

# Deploy to production
ast deploy --env production
\`\`\`

### JSON

\`\`\`json
{
  "name": "customer-insights-engine",
  "version": "1.2.0",
  "dependencies": {
    "@astro/sdk": "^2.0.0",
    "@astro/integrations": "^1.5.0"
  },
  "scripts": {
    "dev": "ast dev",
    "build": "ast build",
    "deploy": "ast deploy"
  }
}
\`\`\`

## Blockquotes

> This is a simple blockquote. Agents should be designed to do one thing well.

> **Note:** Multi-line blockquotes work too.
>
> They can contain **bold**, *italic*, \`code\`, and [links](https://example.com).
>
> > Nested blockquotes are also supported for threading context.

## Tables

| Feature | Free | Pro | Enterprise |
|---------|------|-----|------------|
| Agents | 3 | 25 | Unlimited |
| Integrations | 5 | 20 | Unlimited |
| Observability | Basic | Advanced | Full |
| Support | Community | Priority | Dedicated |
| SSO | - | - | ✓ |

### Alignment

| Left-aligned | Center-aligned | Right-aligned |
|:-------------|:--------------:|--------------:|
| Content | Content | Content |
| More content | More content | More content |
| Even more | Even more | Even more |

## Images

![Placeholder](https://placehold.co/600x200/0f1a19/b8ccc8?text=Agent+Dashboard)

## Horizontal Rules

Content above the rule.

---

Content below the rule.

***

Another section below.

## Footnotes

Here is a statement that needs a citation[^1]. And here is another[^2].

[^1]: This is the first footnote with a detailed explanation.
[^2]: This is the second footnote referencing [external documentation](https://example.com).

## Abbreviations and Special Characters

HTML entities: &copy; 2026 Astro &mdash; All rights reserved. Temperature: 72&deg;F. Price: &dollar;99.

Special characters in text: "quoted text", 'single quotes', ellipsis..., em—dash, en–dash.

## Escaping

Literal asterisks: \\*not bold\\*. Literal backticks: \\\`not code\\\`. Literal hash: \\# not a heading.

## Mixed Content

Here is a paragraph that demonstrates **multiple** *inline* styles together with \`code\`, [links](https://example.com), and ~~strikethrough~~. This is the kind of rich content you would see in a real README.

### Complex Nested Structures

1. **Step one** — Configure your agent
   - Edit \`astroai.yml\` with your settings
   - Supported fields:
     - \`name\` — Agent identifier
     - \`version\` — Semantic version
     - \`integrations\` — Data source connections
   - Example:
     \`\`\`yaml
     name: my-agent
     version: 1.0.0
     \`\`\`
2. **Step two** — Test locally
   > Run \`ast dev\` and verify the agent responds correctly.
3. **Step three** — Deploy
   - Use \`ast deploy --env production\`
   - Monitor via the dashboard

## Long Content

This paragraph is intentionally long to test how the component handles text wrapping and line height across multiple lines. When building agents for production use, it is important to consider how they will handle edge cases, rate limits, and error recovery. A well-designed agent should gracefully degrade when an integration is unavailable, queue retries with exponential backoff, and surface clear error messages to the operator. This ensures reliability even under adverse conditions and maintains trust with the teams that depend on the agent's output.
`;

export const KitchenSink: Story = {
  args: {
    children: KITCHEN_SINK,
  },
};

export const ShortReadme: Story = {
  args: {
    children: `## Overview

A lightweight agent that monitors your GitHub repositories and surfaces actionable insights daily.

## Getting Started

\`\`\`bash
ast install repo-monitor
ast dev
\`\`\`

Configure your repos in \`astroai.yml\` before deploying.

## Configuration

- **repos** — list of repositories to monitor
- **schedule** — cron expression for scan frequency
- **notify** — alert destination (Slack channel or webhook)
`,
  },
};

export const CodeHeavy: Story = {
  args: {
    children: `## Installation

\`\`\`bash
bun add @astro/sdk @astro/integrations
\`\`\`

## Usage

\`\`\`typescript
import { Agent, IntegrationRegistry } from "@astro/sdk";
import { github, slack, linear } from "@astro/integrations";

const registry = new IntegrationRegistry();
registry.register(github({ token: process.env.GITHUB_TOKEN }));
registry.register(slack({ token: process.env.SLACK_TOKEN }));
registry.register(linear({ apiKey: process.env.LINEAR_KEY }));

const agent = new Agent({
  name: "insights-engine",
  integrations: registry,
});

agent.on("scheduled", async (ctx) => {
  const data = await ctx.gather({ timeframe: "24h" });
  const analysis = await ctx.analyze(data);

  if (analysis.score > 0.75) {
    await ctx.notify({
      channel: "#alerts",
      message: analysis.summary,
    });
  }
});
\`\`\`

## API Reference

| Method | Description | Returns |
|--------|-------------|---------|
| \`agent.on(event, handler)\` | Register event handler | \`void\` |
| \`ctx.gather(opts)\` | Collect data from integrations | \`Promise<Data>\` |
| \`ctx.analyze(data)\` | Run analysis pipeline | \`Promise<Analysis>\` |
| \`ctx.notify(opts)\` | Send notification | \`Promise<void>\` |
`,
  },
};
