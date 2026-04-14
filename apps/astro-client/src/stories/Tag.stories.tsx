import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tag } from "@/components/Tag";

const meta = {
  title: "Design System/Primitives/Tag",
  component: Tag,
  args: { children: "Label" },
} satisfies Meta<typeof Tag>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Colors: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Tag>Private</Tag>
      <Tag color="teal">admin</Tag>
      <Tag color="blue">Config change</Tag>
      <Tag color="yellow">Draft</Tag>
      <Tag color="coral">Deprecated</Tag>
    </div>
  ),
};
