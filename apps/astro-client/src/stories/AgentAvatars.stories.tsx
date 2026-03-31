import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentAvatar, type AgentAvatarName } from "@/components/AgentAvatar";

const avatarNames: AgentAvatarName[] = ["agent-avatar-1", "agent-avatar-2", "agent-avatar-3"];

const meta = {
  title: "Features/Agents/AgentAvatars",
  component: AgentAvatar,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof AgentAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    name: "agent-avatar-1",
    size: 52,
  },
};

export const AllAvatars: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      {avatarNames.map((name) => (
        <AgentAvatar key={name} name={name} size={52} className="rounded-lg" />
      ))}
    </div>
  ),
};

export const Stack: Story = {
  render: () => (
    <div className="flex items-center">
      <AgentAvatar name="agent-avatar-1" size={52} className="z-10 relative rounded-lg" />
      <AgentAvatar name="agent-avatar-3" size={52} className="-ml-3 z-20 relative rounded-lg" />
      <AgentAvatar name="agent-avatar-2" size={52} className="-ml-3 z-10 relative rounded-lg" />
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      {[32, 44, 52, 64].map((size) => (
        <div key={size} className="flex items-center gap-3">
          {avatarNames.map((name) => (
            <AgentAvatar key={name} name={name} size={size} className="rounded-lg" />
          ))}
          <span className="text-sm text-muted-foreground ml-2">{size}px</span>
        </div>
      ))}
    </div>
  ),
};
