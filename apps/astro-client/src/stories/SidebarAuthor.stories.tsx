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
    authors: [{ name: "Jane Smith", account: "janesmith" }],
    ownerName: "Jane Smith",
    ownerHandle: "janesmith",
  },
};

export const WithInitials: Story = {
  args: {
    authors: [],
    ownerName: "Acme Corp",
    ownerHandle: "acme",
  },
};
