import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import type { AgentDeployment } from "@/lib/api";

const mockDeployment: AgentDeployment = {
  id: "d-7f3a2b",
  name: "customer-insights-engine",
  display_name: "Customer Insights Engine",
  build_id: "8f406458a3c2d91e",
  namespace: "postman-labs",
  status: "active",
  replicas: 1,
  ready: 1,
  created_at: new Date().toISOString(),
  components: ["agent", "messaging", "collector"],
  external_urls: [
    {
      name: "Agent endpoint",
      url: "https://nexus-4d03eca25bd87d16.agents.astropods.ai",
    },
  ],
  pods: [
    {
      name: "customer-insights-engine-6d8f9b-xkp2t",
      phase: "Running",
      pod_ip: "10.0.0.42",
      age: "2m",
      containers: [
        {
          name: "nexus-agent",
          state: "running",
          ready: true,
          restart_count: 0,
          env: [
            { name: "LOG_LEVEL", value: "info", from: "input" },
            { name: "GRPC_PORT", value: "9090", from: "injected" },
            { name: "MODEL_VERSION", value: "claude-3-7-sonnet-20250219", from: "static" },
            { name: "ANTHROPIC_API_KEY", value: "sk-ant-api03-••••••••••••••••", from: "injected" },
          ],
        },
        {
          name: "nexus-collector",
          state: "running",
          ready: true,
          restart_count: 0,
          env: [
            { name: "SYNC_BATCH_SIZE", value: "100", from: "input" },
            { name: "GITHUB_TOKEN", value: "secret:github/token", from: "injected" },
            { name: "SOURCE_REPO", value: "company/engineering-docs", from: "static" },
          ],
        },
        {
          name: "nexus-messaging",
          state: "running",
          ready: true,
          restart_count: 0,
          env: [
            { name: "GRPC_BIND_PORT", value: "9090", from: "static" },
            { name: "HEALTH_PATH", value: "/healthz", from: "static" },
          ],
        },
      ],
    },
  ],
};

const meta = {
  title: "Features/DeployedAgent/ActiveDetailView",
  component: ActiveDetailView,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof ActiveDetailView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const MonitorTab: Story = {
  args: {
    deployment: mockDeployment,
    account: "postman-labs",
    isPersonal: false,
  },
};

export const PersonalAccount: Story = {
  args: {
    deployment: { ...mockDeployment, display_name: "My Research Agent", name: "my-research-agent" },
    account: "tara",
    isPersonal: true,
  },
};

export const NoPods: Story = {
  args: {
    deployment: { ...mockDeployment, pods: [] },
    account: "postman-labs",
    isPersonal: false,
  },
};

export const DeploymentsTab: Story = {
  args: {
    deployment: mockDeployment,
    account: "postman-labs",
    isPersonal: false,
  },
};
