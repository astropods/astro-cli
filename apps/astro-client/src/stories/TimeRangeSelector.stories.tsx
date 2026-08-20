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

const READOUTS: Record<string, string> = {
  "7d": "Jun 2 – Jun 8",
  "14d": "May 26 – Jun 8",
  "30d": "May 10 – Jun 8",
  "90d": "Mar 11 – Jun 8",
};

function Controlled({
  defaultValue,
  ranges,
  withReadout = false,
}: {
  defaultValue: string;
  ranges?: { key: string; label: string }[];
  withReadout?: boolean;
}) {
  const [value, setValue] = useState(defaultValue);
  return (
    <TimeRangeSelector
      value={value}
      ranges={ranges}
      onChange={setValue}
      leading={withReadout ? READOUTS[value] : undefined}
      size={withReadout ? "lg" : undefined}
    />
  );
}

export const InsightsVariant: Story = {
  render: () => <Controlled defaultValue="30d" ranges={ACTIVITY_RANGES} />,
  name: "Insights (7D / 14D / 30D / All)",
};

export const MonitorVariant: Story = {
  render: () => <Controlled defaultValue="7d" ranges={MONITOR_RANGES} />,
  name: "Monitor (7D / 14D / 30D)",
};

export const WithReadout: Story = {
  render: () => <Controlled defaultValue="30d" ranges={ACTIVITY_RANGES} withReadout />,
  name: "With resolved-window readout",
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
      <div>
        <p className="mb-2 font-mono text-xs text-muted-foreground">Insights with readout</p>
        <Controlled defaultValue="30d" ranges={ACTIVITY_RANGES} withReadout />
      </div>
    </div>
  ),
};
