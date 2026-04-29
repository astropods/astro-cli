import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarAuthor } from "@/components/blueprint-detail/SidebarAuthor";

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

export const MultipleAuthors: Story = {
  args: {
    authors: [
      { name: "Jane Smith", account: "janesmith" },
      { name: "Chris Patty", account: "chrispatty" },
      { name: "Taylor Green", account: "taylorlgreen" },
    ],
    ownerName: "Acme Corp",
    ownerHandle: "acme",
  },
};

export const ManyAuthors: Story = {
  args: {
    authors: [
      { name: "Jane Smith", account: "janesmith" },
      { name: "Chris Patty", account: "chrispatty" },
      { name: "Taylor Green", account: "taylorlgreen" },
      { name: "Sam Rivera", account: "samrivera" },
      { name: "Alex Kim", account: "alexkim" },
    ],
    ownerName: "Acme Corp",
    ownerHandle: "acme",
  },
};
