import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  MemoryRouter,
  Routes,
  Route,
  Outlet,
} from "react-router";

import { RecommendedAgents } from "@/components/RecommendedAgents";
import type { LayoutContext } from "@/components/Layout";

const meta = {
  title: "Components/RecommendedAgents",
  component: RecommendedAgents,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Routes>
          <Route
            element={
              <>
                <Outlet context={{ openAuthModal: () => {} } satisfies LayoutContext} />
              </>
            }
          >
            <Route
              index
              element={
                <div className="max-w-3xl">
                  <Story />
                </div>
              }
            />
          </Route>
        </Routes>
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
    integrations: ["Slack", "Gmail", "Google Drive"],
    categories: ["Analytics", "Support"],
  },
  {
    slug: "sentiment-analyzer",
    account: "astro",
    name: "Sentiment Analyzer",
    description:
      "Monitors customer sentiment across channels and flags critical shifts in real time.",
    integrations: ["Slack", "Gmail"],
    categories: ["Analytics"],
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
