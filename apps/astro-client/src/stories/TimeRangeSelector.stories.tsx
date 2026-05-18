import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { ACTIVITY_RANGES } from "@/components/activity/ranges";

const MONITOR_RANGES = [
  { key: "7d", label: "7D" },
  { key: "14d", label: "14D" },
  { key: "30d", label: "30D" },
];

const meta = {
  title: "Design System/Activity/TimeRangeSelector",
  component: TimeRangeSelector,
  parameters: { layout: "centered" },
  args: {
    value: "30d",
    onChange: () => {},
  },
} satisfies Meta<typeof TimeRangeSelector>;

export default meta;
type Story = StoryObj<typeof meta>;

function Controlled({ defaultValue, ranges }: { defaultValue: string; ranges?: { key: string; label: string }[] }) {
  const [value, setValue] = useState(defaultValue);
  return <TimeRangeSelector value={value} ranges={ranges} onChange={setValue} />;
}

export const InsightsVariant: Story = {
  render: () => <Controlled defaultValue="30d" ranges={ACTIVITY_RANGES} />,
  name: "Insights (7D / 14D / 30D / All)",
};

export const MonitorVariant: Story = {
  render: () => <Controlled defaultValue="7d" ranges={MONITOR_RANGES} />,
  name: "Monitor (7D / 14D / 30D)",
};

export const AllVariants: Story = {
  render: () => (
    <div className="flex flex-col gap-6 items-start">
      <div>
        <p className="mb-2 font-mono text-xs text-muted-foreground">Insights</p>
        <Controlled defaultValue="30d" ranges={ACTIVITY_RANGES} />
      </div>
      <div>
        <p className="mb-2 font-mono text-xs text-muted-foreground">Monitor</p>
        <Controlled defaultValue="7d" ranges={MONITOR_RANGES} />
      </div>
    </div>
  ),
};
