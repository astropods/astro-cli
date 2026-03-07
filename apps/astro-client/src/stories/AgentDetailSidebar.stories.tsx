import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { AgentDetailSidebar } from "@/components/agent-detail/AgentDetailSidebar";
import type { Agent, AccountPublic } from "@/lib/api";

const mockAgent: Agent = {
  name: "customer-insight-engine",
  account: "acme",
  registry: "default",
  versions: [
    {
      build_id: "a1b2c3d4e5f6g7h8",
      version: "1.2.3",
      spec: {
        meta: {
          description: "Analyzes customer feedback.",
          tags: ["analytics"],
        },
      },
      readme: "",
      published_at: "2026-01-15T12:00:00Z",
    },
  ],
};

const mockAccount: AccountPublic = {
  id: "acc-001",
  name: "acme",
  type: "organization",
  owner: {
    first_name: "Jane",
    last_name: "Smith",
    profile_picture_url: "https://i.pravatar.cc/150?u=janesmith",
  },
  created_at: "2025-06-01T00:00:00Z",
  updated_at: "2026-01-15T00:00:00Z",
};

const meta = {
  title: "Features/Agents/AgentDetailSidebar",
  component: AgentDetailSidebar,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="flex justify-end min-h-[600px]">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  // Override hidden lg:block so it always renders in Storybook
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof AgentDetailSidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    agent: mockAgent,
    description:
      "Analyzes customer feedback to surface actionable insights and trends across all channels.",
    integrations: ["Slack", "GitHub"],
    permissions: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
    ],
    initialAccountData: mockAccount,
  },
};

export const Full: Story = {
  args: {
    agent: mockAgent,
    description:
      "This agent monitors your production environment 24/7 and detects anomalies in real-time.",
    integrations: ["Slack", "GitHub", "Linear", "Notion"],
    permissions: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
      "Create Linear issues",
      "Read Notion pages",
      "Send email notifications",
    ],
    initialAccountData: mockAccount,
  },
};

export const Minimal: Story = {
  args: {
    agent: mockAgent,
    description: "",
    integrations: [],
    permissions: [],
    initialAccountData: mockAccount,
  },
};

export const NoAvatar: Story = {
  args: {
    agent: mockAgent,
    description: "A lightweight monitoring agent.",
    integrations: ["Slack"],
    permissions: ["Read Slack messages"],
    initialAccountData: {
      ...mockAccount,
      owner: { first_name: "Acme", last_name: "Corp" },
    },
  },
};

export const BuildIdVersion: Story = {
  args: {
    agent: {
      ...mockAgent,
      versions: [
        {
          ...mockAgent.versions[0],
          version: undefined,
        },
      ],
    },
    description: "Uses a build ID instead of semver.",
    integrations: ["GitHub"],
    permissions: ["Access GitHub repositories"],
    initialAccountData: mockAccount,
  },
};
