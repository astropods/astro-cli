import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { RecommendedAgents } from "@/components/RecommendedAgents";

const meta = {
  title: "Features/Agents/RecommendedAgents",
  component: RecommendedAgents,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-3xl">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof RecommendedAgents>;

export default meta;
type Story = StoryObj<typeof meta>;

const sampleAgents = [
  {
    slug: "ticket-scanner",
    account: "astro",
    name: "Ticket Scanner",
    description:
      "Surfaces actionable feedback from support tickets, reviews, and user conversations to guide product decisions.",
  },
  {
    slug: "sentiment-analyzer",
    account: "astro",
    name: "Sentiment Analyzer",
    description:
      "Monitors customer sentiment across channels and flags critical shifts in real time.",
  },
];

export const Default: Story = {
  args: {
    agents: sampleAgents,
  },
};

export const SingleAgent: Story = {
  args: {
    agents: [sampleAgents[0]],
  },
};

export const Empty: Story = {
  args: {
    agents: [],
  },
};
