import type { Meta, StoryObj } from "@storybook/react-vite";
import { UserAvatar } from "@/components/UserAvatar";
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
  title: "Components/UserAvatar",
  component: UserAvatar,
  args: {
    user: baseUser,
  },
} satisfies Meta<typeof UserAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithPhoto: Story = {};

export const Initials: Story = {
  args: {
    user: { ...baseUser, profile_picture_url: undefined },
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

export const CustomSize: Story = {
  args: {
    className: "size-12 text-base",
    user: { ...baseUser, profile_picture_url: undefined },
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <UserAvatar user={baseUser} className="size-6" />
      <UserAvatar user={baseUser} />
      <UserAvatar user={baseUser} className="size-10" />
      <UserAvatar user={baseUser} className="size-12" />
    </div>
  ),
};
