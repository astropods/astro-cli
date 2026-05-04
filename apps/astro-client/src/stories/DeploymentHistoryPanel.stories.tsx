import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DeploymentHistoryPanelContent,
  type DeploymentHistoryPanelContentProps,
} from "@/components/agent-detail/deployments/DeploymentHistoryPanel";
import { DeploymentTile } from "@/components/agent-detail/deployments/DeploymentTile";
import { StarField } from "@/components/agent-detail/starfield/StarField";
import type { AgentDeployment } from "@/lib/api";

function mockDeployment(overrides: Partial<AgentDeployment> = {}): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    build_id: "a1b2c3d4",
    namespace: "ns",
    status: "running",
    replicas: 1,
    ready: 1,
    created_at: new Date().toISOString(),
    components: ["agent"],
    ...overrides,
  };
}

const meta = {
  title: "Features/Agent Detail/DeploymentHistoryPanel",
  component: DeploymentHistoryPanelContent,
  decorators: [
    (Story) => (
      <div className="relative flex h-[600px] items-stretch justify-end bg-surface p-3">
        <StarField />
        <div className="relative z-10">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<DeploymentHistoryPanelContentProps>;

export default meta;
type Story = StoryObj<DeploymentHistoryPanelContentProps>;

export const Empty: Story = {
  args: {},
};

export const SingleActive: Story = {
  args: {
    children: (
      <DeploymentTile
        name="Add streaming support for tool calls"
        source="github"
        branch="main"
        buildId="f7e8d9c0b1a2"
        deployedAt={new Date(Date.now() - 1000 * 60 * 10).toISOString()}
        active
        deployment={mockDeployment()}
      />
    ),
  },
};

export const WithHistory: Story = {
  args: {
    children: (
      <>
        <DeploymentTile
          name="Add streaming support for tool calls"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 10).toISOString()}
          active
          deployment={mockDeployment()}
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="a1b2c3d4e5f6"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="deadbeef1234"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString()}
        />
        <DeploymentTile
          name="Fix rate limiting on /v1/chat endpoint"
          source="github"
          branch="fix/rate-limit"
          buildId="cafe0123abcd"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 3).toISOString()}
        />
        <DeploymentTile
          name="Initial deployment"
          source="direct"
          buildId="0000abcd1234"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 14).toISOString()}
        />
      </>
    ),
  },
};

export const DirectPushesOnly: Story = {
  args: {
    children: (
      <>
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="abcdef12"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60).toISOString()}
          active
          deployment={mockDeployment()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="12345678"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 48).toISOString()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="98765432"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 7).toISOString()}
        />
      </>
    ),
  },
};
