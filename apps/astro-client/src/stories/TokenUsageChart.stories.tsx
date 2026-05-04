import type { Meta, StoryObj } from "@storybook/react-vite";
import { TokenUsageChart, type TokenUsageChartProps } from "@/components/agent-detail/charts/TokenUsageChart";
import { CHART_COLORS, type TokenUsageBar } from "@/components/agent-detail/charts/chart-utils";

function bar(label: string, input: number, output: number): TokenUsageBar {
  return { label, inputTokens: input, outputTokens: output };
}

function generate7dBars(): TokenUsageBar[] {
  const today = new Date();
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (6 - i));
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const input = Math.round(20_000 + Math.random() * 80_000);
    const output = Math.round(input * (0.35 + Math.random() * 0.25));
    return bar(label, input, output);
  });
}

function generate14dBars(): TokenUsageBar[] {
  const today = new Date();
  return Array.from({ length: 14 }, (_, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (13 - i));
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const input = Math.round(15_000 + Math.random() * 60_000);
    const output = Math.round(input * (0.3 + Math.random() * 0.3));
    return bar(label, input, output);
  });
}

function generate30dBars(): TokenUsageBar[] {
  const today = new Date();
  return Array.from({ length: 30 }, (_, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (29 - i));
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const curve = Math.sin((i / 30) * Math.PI * 2) * 0.5 + 0.5;
    const base = 10_000 + curve * 90_000;
    const input = Math.round(base * (0.8 + Math.random() * 0.4));
    const output = Math.round(input * (0.35 + Math.random() * 0.25));
    return bar(label, input, output);
  });
}

function generateSparseBars(): TokenUsageBar[] {
  const today = new Date();
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (6 - i));
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const hasActivity = i === 2 || i === 5;
    const input = hasActivity ? Math.round(30_000 + Math.random() * 50_000) : 0;
    const output = hasActivity ? Math.round(input * 0.4) : 0;
    return bar(label, input, output);
  });
}

const defaultColors = CHART_COLORS.dark;

const meta = {
  title: "Features/Agent Detail/TokenUsageChart",
  component: TokenUsageChart,
  decorators: [
    (Story) => (
      <div className="min-h-[480px] bg-surface p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<TokenUsageChartProps>;

export default meta;
type Story = StoryObj<TokenUsageChartProps>;

export const SevenDays: Story = {
  args: { bars: generate7dBars(), colors: defaultColors },
};

export const FourteenDays: Story = {
  args: { bars: generate14dBars(), colors: defaultColors },
};

export const ThirtyDays: Story = {
  args: { bars: generate30dBars(), colors: defaultColors },
};

export const SparseActivity: Story = {
  args: { bars: generateSparseBars(), colors: defaultColors },
};

export const FreshAgent: Story = {
  name: "Fresh Agent (1 day of data)",
  args: {
    bars: (() => {
      const bars = generate7dBars().map((b) => ({ ...b, inputTokens: 0, outputTokens: 0 }));
      bars[bars.length - 1] = { ...bars[bars.length - 1], inputTokens: 3200, outputTokens: 1100 };
      return bars;
    })(),
    colors: defaultColors,
  },
};

export const Loading: Story = {
  args: { bars: [], colors: defaultColors, loading: true },
};

export const Empty: Story = {
  args: { bars: [], colors: defaultColors },
};

export const LightMode: Story = {
  name: "Light Mode",
  args: { bars: generate7dBars(), colors: CHART_COLORS.light },
};
