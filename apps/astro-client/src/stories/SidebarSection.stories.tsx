import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarSection } from "@/components/agent-detail/SidebarSection";

const meta = {
  title: "Features/Agents/Sidebar/SidebarSection",
  component: SidebarSection,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SidebarSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Section Title",
    children: (
      <p className="text-sm text-foreground">
        Any content can go here — text, lists, cards, etc.
      </p>
    ),
  },
};
