import type { Meta, StoryObj } from "@storybook/react-vite";
import { UserAvatar } from "@/components/UserAvatar";

const meta = {
  title: "Design System/Composites/UserAvatar",
  component: UserAvatar,
  args: {
    handle: "janesmith",
    name: "Jane Doe",
  },
} satisfies Meta<typeof UserAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <UserAvatar handle="janesmith" name="Jane Doe" className="size-6 rounded" />
      <UserAvatar handle="janesmith" name="Jane Doe" />
      <UserAvatar handle="janesmith" name="Jane Doe" className="size-10" />
      <UserAvatar handle="janesmith" name="Jane Doe" className="size-12" />
    </div>
  ),
};

export const DeterministicAssignment: Story = {
  render: () => (
    <div className="flex flex-col gap-4 p-2">
      <p className="text-body-sm text-muted-foreground">
        A handle always resolves to the same avatar URL. Handles with no uploaded image, as here,
        land on the shared placeholder.
      </p>
      <div className="flex flex-wrap gap-3">
        {Array.from({ length: 10 }, (_, i) => {
          const handle = `user-${i + 1}`;
          return (
            <div key={handle} className="flex flex-col items-center gap-1">
              <UserAvatar handle={handle} name={`User ${i + 1}`} className="size-10" />
              <span className="text-mono-sm font-mono text-faint-foreground">{handle}</span>
            </div>
          );
        })}
      </div>
    </div>
  ),
};
