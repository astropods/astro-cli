import type { Meta, StoryObj } from "@storybook/react-vite";
import { NetworkFlowsTable, type NetworkFlowsTableProps } from "@/components/agent-detail/network/NetworkFlowsTable";
import type { NetworkFlow } from "@/lib/api";

function flow(
  peer: string,
  peer_kind: NetworkFlow["peer_kind"],
  requests: number,
  errors: number,
  p50: number | null,
  p95: number | null,
  statusCodes?: Record<string, number>,
): NetworkFlow {
  return {
    peer,
    peer_kind,
    request_count: requests,
    error_count: errors,
    error_rate: requests > 0 ? errors / requests : 0,
    latency_p50_ms: p50,
    latency_p95_ms: p95,
    bytes_total: requests * 1024,
    status_codes: statusCodes,
  };
}

const inboundFlows: NetworkFlow[] = [
  flow("/api/messages", "route", 4820, 18, 12.4, 84, { "2xx": 4802, "4xx": 12, "5xx": 6 }),
  flow("/health", "route", 12400, 0, 1.2, 4.8, { "2xx": 12400 }),
  flow("/api/users/{id}", "route", 1820, 7, 22.1, 142, { "2xx": 1801, "3xx": 12, "4xx": 7 }),
  flow("/webhooks/slack", "route", 340, 28, 64, 480, { "2xx": 312, "5xx": 28 }),
  flow("/admin/status", "route", 12, 0, 8.5, 14, { "2xx": 12 }),
  flow("/", "route", 9, 0, 4.2, 6.8, { "2xx": 9 }),
];

const outboundFlows: NetworkFlow[] = [
  flow("api.openai.com", "address", 1842, 9, 412, 1820, { "2xx": 1833, "5xx": 9 }),
  flow("api.anthropic.com", "address", 920, 4, 380, 1640, { "2xx": 916, "5xx": 4 }),
  flow("hooks.slack.com", "address", 421, 0, 86, 240, { "2xx": 421 }),
  flow("api.github.com", "address", 78, 2, 110, 320, { "2xx": 76, "4xx": 2 }),
];

const databaseFlows: NetworkFlow[] = [
  flow("postgresql", "db_system", 9842, 0, 2.1, 14.8),
  flow("redis", "db_system", 2104, 0, 0.6, 3.2),
];

const meta = {
  title: "Features/Agent Detail/NetworkFlowsTable",
  component: NetworkFlowsTable,
  decorators: [
    (Story) => (
      <div className="w-[56rem] bg-stone-950 p-4">
        <Story />
      </div>
    ),
  ],
  argTypes: {
    direction: { control: "select", options: ["inbound", "outbound", "database"] },
    loading: { control: "boolean" },
  },
} satisfies Meta<NetworkFlowsTableProps>;

export default meta;
type Story = StoryObj<NetworkFlowsTableProps>;

export const Inbound: Story = {
  args: { direction: "inbound", flows: inboundFlows },
};

export const Outbound: Story = {
  args: { direction: "outbound", flows: outboundFlows },
};

export const Database: Story = {
  args: { direction: "database", flows: databaseFlows },
};

export const Empty: Story = {
  args: { direction: "outbound", flows: [] },
};

export const Loading: Story = {
  args: { direction: "inbound", flows: [], loading: true },
};

export const ManyRows: Story = {
  args: {
    direction: "inbound",
    flows: [
      ...inboundFlows,
      flow("/api/agents", "route", 820, 3, 18, 96, { "2xx": 817, "4xx": 3 }),
      flow("/api/agents/{id}/run", "route", 612, 1, 240, 1820, { "2xx": 611, "5xx": 1 }),
      flow("/api/agents/{id}/logs", "route", 412, 0, 12, 88, { "2xx": 412 }),
      flow("/api/agents/{id}/metrics", "route", 184, 0, 9, 42, { "2xx": 184 }),
      flow("/api/auth/refresh", "route", 92, 4, 14, 32, { "2xx": 88, "4xx": 4 }),
      flow("/api/auth/logout", "route", 41, 0, 6, 18, { "2xx": 41 }),
    ],
  },
};

export const WithUnknownPeer: Story = {
  args: {
    direction: "outbound",
    flows: [
      ...outboundFlows,
      flow("(unknown)", "address", 14, 14, 1840, 4800, { "5xx": 14 }),
    ],
  },
};

export const VeryHighErrors: Story = {
  args: {
    direction: "outbound",
    flows: [
      flow("api.openai.com", "address", 320, 290, 4200, 18000, { "2xx": 30, "5xx": 290 }),
      flow("hooks.slack.com", "address", 84, 84, 60, 120, { "5xx": 84 }),
    ],
  },
};
