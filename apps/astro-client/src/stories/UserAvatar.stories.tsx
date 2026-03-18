import type { Meta, StoryObj } from "@storybook/react-vite";
import { UserAvatar } from "@/components/UserAvatar";
import { PRESET_AVATARS } from "@/lib/presetAvatars";
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
  title: "Design System/Composites/UserAvatar",
  component: UserAvatar,
  args: {
    user: baseUser,
  },
} satisfies Meta<typeof UserAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithPhoto: Story = {};

export const PresetFallback: Story = {
  args: {
    user: { ...baseUser, profile_picture_url: undefined },
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <UserAvatar user={{ ...baseUser, profile_picture_url: undefined }} className="size-6 rounded" />
      <UserAvatar user={{ ...baseUser, profile_picture_url: undefined }} />
      <UserAvatar user={{ ...baseUser, profile_picture_url: undefined }} className="size-10" />
      <UserAvatar user={{ ...baseUser, profile_picture_url: undefined }} className="size-12" />
    </div>
  ),
};

export const DeterministicAssignment: Story = {
  render: () => (
    <div className="flex flex-col gap-4 p-2">
      <p className="text-body-sm text-muted-foreground">
        Each user ID consistently maps to the same avatar across all sessions.
      </p>
      <div className="flex flex-wrap gap-3">
        {PRESET_AVATARS.map((_, i) => {
          const user: User = {
            ...baseUser,
            id: `user-${i + 1}`,
            profile_picture_url: undefined,
          };
          return (
            <div key={user.id} className="flex flex-col items-center gap-1">
              <UserAvatar user={user} className="size-10" />
              <span className="text-mono-sm font-mono text-faint-foreground">{user.id}</span>
            </div>
          );
        })}
      </div>
    </div>
  ),
};
