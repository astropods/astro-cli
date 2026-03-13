import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentDetailContent } from "@/components/agent-detail/AgentDetailContent";

const longReadme = [
  "# API Changelog Writer",
  "",
  "Intro block that gets skipped by the temporary heuristic.",
  "",
  "## What this agent does",
  "",
  "- Watches your API changelog updates",
  "- Drafts release summaries",
  "- Suggests migration notes",
  "",
  "## Usage",
  "",
  "Provide a changelog URL and your target audience.",
  "",
  "## Output format",
  "",
  "Markdown with sections for highlights, risks, and rollout plan.",
].join("\n");

const meta = {
  title: "Features/Agents/AgentDetailContent",
  component: AgentDetailContent,
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-5xl">
        <Story />
      </div>
    ),
  ],
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof AgentDetailContent>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    account: "sohumonlocal",
    name: "api-changelog-writer",
    summary:
      "Summarizes API changes into concise release notes and migration guidance for product and engineering teams.",
    categories: ["PRODUCTIVITY", "ENGINEERING"],
    readme: longReadme,
  },
};

export const NoSummary: Story = {
  args: {
    account: "sohumonlocal",
    name: "api-changelog-writer",
    categories: [],
    readme: "## ReadMe\n\nMinimal content block.",
  },
};
