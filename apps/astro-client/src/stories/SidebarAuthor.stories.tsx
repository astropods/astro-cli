import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarAuthor } from "@/components/agent-detail/SidebarAuthor";

const meta = {
  title: "Features/Agents/Sidebar/SidebarAuthor",
  component: SidebarAuthor,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SidebarAuthor>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithAvatar: Story = {
  args: {
    name: "Jane Smith",
    handle: "janesmith",
    profilePictureUrl: "https://i.pravatar.cc/150?u=janesmith",
  },
};

export const WithInitials: Story = {
  args: {
    name: "Acme Corp",
    handle: "acme",
  },
};
