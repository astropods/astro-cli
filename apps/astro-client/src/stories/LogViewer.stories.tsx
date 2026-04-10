import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import type { LogEntry } from "@/lib/log-utils";

const SAMPLE_LOGS: LogEntry[] = [
  { timestamp: "2024-03-15T10:23:01.123Z", level: null, message: "Starting agent runtime v2.4.1" },
  { timestamp: "2024-03-15T10:23:01.456Z", level: null, message: "Connecting to model endpoint claude-3-7-sonnet-20250219" },
  { timestamp: "2024-03-15T10:23:02.001Z", level: null, message: "Connected and ready to serve requests" },
  { timestamp: "2024-03-15T10:23:05.312Z", level: null, message: "Received request id=req-001 from user=alice" },
  { timestamp: "2024-03-15T10:23:05.401Z", level: null, message: "Tool call: search_knowledge_base query=\"quarterly revenue\"" },
  { timestamp: "2024-03-15T10:23:05.890Z", level: null, message: "Tool result: 3 documents retrieved" },
  { timestamp: "2024-03-15T10:23:06.210Z", level: null, message: "Response generated in 809ms tokens=142" },
  { timestamp: "2024-03-15T10:23:11.003Z", level: null, message: "Received request id=req-002 from user=bob" },
  { timestamp: "2024-03-15T10:23:11.512Z", level: "WARN", message: "Warning: retry attempt 1 for tool=send_email" },
  { timestamp: "2024-03-15T10:23:12.004Z", level: "WARN", message: "Warning: retry attempt 2 for tool=send_email" },
  { timestamp: "2024-03-15T10:23:12.801Z", level: "ERROR", message: "Error: failed to deliver email — SMTP connection refused" },
  { timestamp: "2024-03-15T10:23:12.900Z", level: null, message: "Request id=req-002 completed with error" },
  { timestamp: "2024-03-15T10:23:20.100Z", level: null, message: "Received request id=req-003 from user=carol" },
  { timestamp: "2024-03-15T10:23:20.500Z", level: null, message: "Tool call: list_files path=\"/reports\"" },
  { timestamp: "2024-03-15T10:23:20.612Z", level: null, message: "Tool result: success 8 files found" },
  { timestamp: "2024-03-15T10:23:21.300Z", level: null, message: "Response generated in 800ms tokens=88" },
];

const meta = {
  title: "Features/Logs/LogViewer",
  component: LogViewer,
  parameters: { layout: "padded" },
  args: {
    logs: [],
    timeRange: "15m" as LogTimeRange,
    onTimeRangeChange: () => {},
  },
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

function Controlled(args: Omit<React.ComponentProps<typeof LogViewer>, "timeRange" | "onTimeRangeChange" | "isLive" | "onLiveToggle"> & { withLive?: boolean; startLive?: boolean }) {
  const [timeRange, setTimeRange] = useState<LogTimeRange>("15m");
  const [isLive, setIsLive] = useState(args.startLive ?? false);
  const { withLive, startLive, ...rest } = args;
  return (
    <LogViewer
      {...rest}
      timeRange={timeRange}
      onTimeRangeChange={setTimeRange}
      isLive={withLive ? isLive : undefined}
      onLiveToggle={withLive ? () => setIsLive((v) => !v) : undefined}
    />
  );
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

export const Live: Story = {
  render: () => <Controlled logs={SAMPLE_LOGS} withLive startLive />,
};

export const LiveConnecting: Story = {
  render: () => <Controlled logs={[]} withLive startLive isLoading />,
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
