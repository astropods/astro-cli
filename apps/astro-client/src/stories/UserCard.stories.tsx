import type { Meta, StoryObj } from "@storybook/react-vite";
import { UserCard } from "@/components/UserCard";
import type { User } from "@/lib/api";

const baseUser: User = {
  id: "user-1",
  email: "jane.doe@example.com",
  first_name: "Jane",
  last_name: "Doe",
  email_verified: true,
  profile_picture_url: "https://i.pravatar.cc/128?u=jane",
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:00:00Z",
};

const meta = {
  title: "Design System/Composites/UserCard",
  component: UserCard,
  decorators: [
    (Story) => (
      <div className="w-64">
        <Story />
      </div>
    ),
  ],
  args: {
    user: baseUser,
    handle: "acct-1",
    onSignOut: () => console.log("Sign out clicked"),
  },
} satisfies Meta<typeof UserCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithPhoto: Story = {};

export const WithPresetAvatar: Story = {
  args: {
    user: { ...baseUser, profile_picture_url: undefined },
  },
};

export const LongName: Story = {
  args: {
    user: {
      ...baseUser,
      first_name: "Alexandria",
      last_name: "Constantinopolous",
      email: "alexandria.constantinopolous@longdomainname.example.com",
      profile_picture_url: undefined,
    },
  },
};

export const EmailOnly: Story = {
  args: {
    user: {
      ...baseUser,
      first_name: undefined,
      last_name: undefined,
      profile_picture_url: undefined,
    },
  },
};
