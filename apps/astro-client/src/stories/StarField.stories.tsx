import type { Meta, StoryObj } from "@storybook/react-vite";
import { StarField } from "@/components/agent-detail/starfield/StarField";

const meta = {
  title: "Features/Agent Detail/StarField",
  component: StarField,
  decorators: [
    (Story) => (
      <div className="relative h-[500px] w-full overflow-hidden">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StarField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
