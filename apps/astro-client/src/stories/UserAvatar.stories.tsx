import type { Meta, StoryObj } from "@storybook/react-vite";
import { UserAvatar } from "@/components/UserAvatar";
import { PRESET_AVATARS } from "@/lib/presetAvatars";

const meta = {
  title: "Design System/Composites/UserAvatar",
  component: UserAvatar,
  args: {
    accountId: "acct-1",
    name: "Jane Doe",
    profilePictureUrl: "https://i.pravatar.cc/128?u=jane",
  },
} satisfies Meta<typeof UserAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WithPhoto: Story = {};

export const PresetFallback: Story = {
  args: {
    profilePictureUrl: undefined,
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <UserAvatar accountId="acct-1" name="Jane Doe" className="size-6 rounded" />
      <UserAvatar accountId="acct-1" name="Jane Doe" />
      <UserAvatar accountId="acct-1" name="Jane Doe" className="size-10" />
      <UserAvatar accountId="acct-1" name="Jane Doe" className="size-12" />
    </div>
  ),
};

export const DeterministicAssignment: Story = {
  render: () => (
    <div className="flex flex-col gap-4 p-2">
      <p className="text-body-sm text-muted-foreground">
        Each account ID consistently maps to the same avatar across all sessions.
      </p>
      <div className="flex flex-wrap gap-3">
        {PRESET_AVATARS.map((_, i) => {
          const id = `acct-${i + 1}`;
          return (
            <div key={id} className="flex flex-col items-center gap-1">
              <UserAvatar accountId={id} name={`User ${i + 1}`} className="size-10" />
              <span className="text-mono-sm font-mono text-faint-foreground">{id}</span>
            </div>
          );
        })}
      </div>
    </div>
  ),
};
