import type { Meta, StoryObj } from "@storybook/react-vite";
import { MetricCard } from "@/components/MetricCard";

const meta = {
  title: "Features/Agents/MetricCard",
  component: MetricCard,
  argTypes: {
    higherIsBetter: { control: "boolean" },
    loading: { control: "boolean" },
    trendLoading: { control: "boolean" },
    trend: { control: "number" },
  },
} satisfies Meta<typeof MetricCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: "Requests today",
    value: "1,284",
    trend: 12,
    higherIsBetter: true,
  },
};

export const NegativeTrend: Story = {
  args: {
    label: "Error rate",
    value: "3.2%",
    trend: 18,
    higherIsBetter: false,
  },
};

export const GoodNegativeTrend: Story = {
  args: {
    label: "P95 latency",
    value: "420ms",
    trend: -8,
    higherIsBetter: false,
  },
};

export const NoTrend: Story = {
  args: {
    label: "Tokens today",
    value: "84,210",
  },
};

export const Loading: Story = {
  args: {
    label: "Requests today",
    value: "—",
    loading: true,
    trendLoading: true,
  },
};

export const ValueLoadedTrendPending: Story = {
  args: {
    label: "Requests today",
    value: "1,284",
    trendLoading: true,
  },
};

export const AllStates: Story = {
  args: { label: "Requests today", value: "1,284" },
  render: () => (
    <div className="grid grid-cols-2 gap-3 max-w-lg">
      <MetricCard label="Requests today" value="1,284" trend={12} higherIsBetter />
      <MetricCard label="Error rate" value="3.2%" trend={18} higherIsBetter={false} />
      <MetricCard label="P95 latency" value="420ms" trend={-8} higherIsBetter={false} />
      <MetricCard label="Tokens today" value="84,210" />
      <MetricCard label="Loading" value="—" loading trendLoading />
      <MetricCard label="Trend loading" value="1,284" trendLoading />
    </div>
  ),
};
