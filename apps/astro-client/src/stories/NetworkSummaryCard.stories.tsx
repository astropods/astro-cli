import type { Meta, StoryObj } from "@storybook/react-vite";
import { NetworkSummaryCard, type NetworkSummaryCardProps } from "@/components/agent-detail/network/NetworkSummaryCard";
import { CHART_COLORS } from "@/components/agent-detail/charts/chart-utils";
import type { NetworkDirectionSummary } from "@/lib/api";

function mockSummary(overrides: Partial<NetworkDirectionSummary> = {}): NetworkDirectionSummary {
  return {
    request_count: 12480,
    error_count: 73,
    error_rate: 0.00585,
    latency_p50_ms: 18.4,
    latency_p95_ms: 142,
    latency_p99_ms: 580,
    unique_peer_count: 14,
    bytes_total: 8421360,
    ...overrides,
  };
}

const meta = {
  title: "Features/Agent Detail/NetworkSummaryCard",
  component: NetworkSummaryCard,
  decorators: [
    (Story) => (
      <div className="w-[20rem] bg-stone-950 p-4">
        <Story />
      </div>
    ),
  ],
  args: {
    colors: CHART_COLORS.dark,
  },
} satisfies Meta<NetworkSummaryCardProps>;

export default meta;
type Story = StoryObj<NetworkSummaryCardProps>;

export const Inbound: Story = {
  args: {
    title: "Inbound",
    summary: mockSummary(),
  },
};

export const OutboundHighLatency: Story = {
  args: {
    title: "Outbound",
    summary: mockSummary({
      request_count: 3210,
      error_count: 12,
      error_rate: 0.00374,
      latency_p50_ms: 220,
      latency_p95_ms: 1820,
      latency_p99_ms: 3400,
      unique_peer_count: 4,
    }),
  },
};

export const DatabaseTinyLatency: Story = {
  args: {
    title: "Database",
    summary: mockSummary({
      request_count: 9842,
      error_count: 0,
      error_rate: 0,
      latency_p50_ms: 2.1,
      latency_p95_ms: 14.8,
      latency_p99_ms: 42.3,
      unique_peer_count: 1,
      bytes_total: 0,
    }),
  },
};

export const HighErrorRate: Story = {
  args: {
    title: "Outbound",
    summary: mockSummary({
      request_count: 580,
      error_count: 87,
      error_rate: 0.15,
      latency_p50_ms: 412,
      latency_p95_ms: 2400,
      latency_p99_ms: 5800,
      unique_peer_count: 2,
    }),
  },
};

export const VeryLowErrorRate: Story = {
  args: {
    title: "Inbound",
    summary: mockSummary({
      request_count: 1_240_000,
      error_count: 3,
      error_rate: 0.0000024,
    }),
  },
};

export const NoTraffic: Story = {
  args: {
    title: "Outbound",
    summary: mockSummary({
      request_count: 0,
      error_count: 0,
      error_rate: 0,
      latency_p50_ms: null,
      latency_p95_ms: null,
      latency_p99_ms: null,
      unique_peer_count: 0,
      bytes_total: 0,
    }),
  },
};

export const CustomEmptyMessage: Story = {
  args: {
    title: "Outbound",
    summary: undefined,
    emptyMessage: "Outbound HTTPS not instrumented for Bun runtimes yet",
  },
};

export const Loading: Story = {
  args: {
    title: "Inbound",
    summary: undefined,
    loading: true,
  },
};

export const ThreeUp: Story = {
  args: { title: "", summary: undefined, colors: CHART_COLORS.dark },
  decorators: [
    () => (
      <div className="grid w-[60rem] grid-cols-3 gap-4 bg-stone-950 p-4">
        <NetworkSummaryCard title="Inbound" summary={mockSummary()} colors={CHART_COLORS.dark} />
        <NetworkSummaryCard
          title="Outbound"
          summary={mockSummary({
            request_count: 3210,
            error_count: 12,
            error_rate: 0.00374,
            latency_p50_ms: 65,
            latency_p95_ms: 412,
            latency_p99_ms: 1230,
            unique_peer_count: 6,
          })}
          colors={CHART_COLORS.dark}
        />
        <NetworkSummaryCard
          title="Database"
          summary={mockSummary({
            request_count: 9842,
            error_count: 0,
            error_rate: 0,
            latency_p50_ms: 2.1,
            latency_p95_ms: 14.8,
            latency_p99_ms: 42.3,
            unique_peer_count: 1,
            bytes_total: 0,
          })}
          colors={CHART_COLORS.dark}
        />
      </div>
    ),
  ],
};
