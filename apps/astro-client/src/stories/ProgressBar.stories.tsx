import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ProgressBar,
  type ProgressBarTone,
} from "@/components/ui/progress-bar";

const meta = {
  title: "Design System/Primitives/ProgressBar",
  component: ProgressBar,
  args: {
    "aria-label": "Progress",
    value: 64,
    max: 100,
    tone: "primary",
    size: "sm",
  },
  argTypes: {
    tone: {
      control: "select",
      options: ["primary", "success", "warning", "destructive", "muted"],
    },
    size: {
      control: "select",
      options: ["xs", "sm"],
    },
  },
  decorators: [
    (Story) => (
      <div className="w-72">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ProgressBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

const tones: ProgressBarTone[] = [
  "primary",
  "success",
  "warning",
  "destructive",
  "muted",
];

export const AllTones: Story = {
  render: () => (
    <div className="flex w-72 flex-col gap-5">
      {tones.map((tone, index) => (
        <div key={tone}>
          <div className="mb-2 text-body-sm capitalize text-muted-foreground">
            {tone}
          </div>
          <ProgressBar
            aria-label={`${tone} progress`}
            value={35 + index * 12}
            tone={tone}
          />
        </div>
      ))}
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex w-72 flex-col gap-5">
      <ProgressBar aria-label="Extra-small progress" value={72} size="xs" />
      <ProgressBar aria-label="Small progress" value={72} size="sm" />
    </div>
  ),
};
