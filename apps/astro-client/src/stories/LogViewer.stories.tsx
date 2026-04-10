import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";

const SAMPLE_LOGS = [
  "2024-03-15T10:23:01.123Z Starting agent runtime v2.4.1",
  "2024-03-15T10:23:01.456Z Connecting to model endpoint claude-3-7-sonnet-20250219",
  "2024-03-15T10:23:02.001Z Connected and ready to serve requests",
  "2024-03-15T10:23:05.312Z Received request id=req-001 from user=alice",
  "2024-03-15T10:23:05.401Z Tool call: search_knowledge_base query=\"quarterly revenue\"",
  "2024-03-15T10:23:05.890Z Tool result: 3 documents retrieved",
  "2024-03-15T10:23:06.210Z Response generated in 809ms tokens=142",
  "2024-03-15T10:23:11.003Z Received request id=req-002 from user=bob",
  "2024-03-15T10:23:11.512Z Warning: retry attempt 1 for tool=send_email",
  "2024-03-15T10:23:12.004Z Warning: retry attempt 2 for tool=send_email",
  "2024-03-15T10:23:12.801Z Error: failed to deliver email — SMTP connection refused",
  "2024-03-15T10:23:12.900Z Request id=req-002 completed with error",
  "2024-03-15T10:23:20.100Z Received request id=req-003 from user=carol",
  "2024-03-15T10:23:20.500Z Tool call: list_files path=\"/reports\"",
  "2024-03-15T10:23:20.612Z Tool result: success 8 files found",
  "2024-03-15T10:23:21.300Z Response generated in 800ms tokens=88",
];

const meta = {
  title: "Features/Logs/LogViewer",
  component: LogViewer,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ height: "500px", display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LogViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

function Controlled(args: Omit<React.ComponentProps<typeof LogViewer>, "timeRange" | "onTimeRangeChange">) {
  const [timeRange, setTimeRange] = useState<LogTimeRange>("15m");
  return <LogViewer {...args} timeRange={timeRange} onTimeRangeChange={setTimeRange} />;
}

export const WithLogs: Story = {
  render: () => <Controlled logs={SAMPLE_LOGS} />,
};

export const Empty: Story = {
  render: () => <Controlled logs={[]} />,
};

export const Loading: Story = {
  render: () => <Controlled logs={[]} isLoading />,
};

export const Compact: Story = {
  render: () => <Controlled logs={SAMPLE_LOGS} isCompact />,
  decorators: [
    (Story) => (
      <div style={{ height: "500px", width: "480px", display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
};

export const WithLeading: Story = {
  render: () => (
    <Controlled
      logs={SAMPLE_LOGS}
      leading={
        <select className="h-8 px-2 text-sm border border-border rounded bg-popover font-sans">
          <option>agent / app</option>
          <option>agent / messaging</option>
        </select>
      }
    />
  ),
};
