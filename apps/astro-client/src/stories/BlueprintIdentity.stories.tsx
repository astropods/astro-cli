import type { Meta, StoryObj } from "@storybook/react-vite"

import { BlueprintIdentity } from "@/components/BlueprintIdentity"

const meta = {
  title: "Features/Agents/BlueprintIdentity",
  component: BlueprintIdentity,
  parameters: { layout: "padded" },
  argTypes: {
    size: { control: "number" },
  },
} satisfies Meta<typeof BlueprintIdentity>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    account: "acme-corp",
    name: "research-assistant",
  },
}

export const SmallSize: Story = {
  name: "Small",
  args: {
    account: "acme-corp",
    name: "research-assistant",
    size: 48,
  },
}

export const LargeSize: Story = {
  name: "Large",
  args: {
    account: "acme-corp",
    name: "research-assistant",
    size: 256,
  },
}

export const DifferentSeeds: Story = {
  args: {
    account: "alice",
    name: "code-reviewer",
  },
  render: () => (
    <div className="flex flex-wrap gap-4">
      {[
        { account: "alice", name: "code-reviewer" },
        { account: "bob", name: "data-pipeline" },
        { account: "charlie", name: "slack-bot" },
        { account: "acme", name: "customer-support" },
        { account: "postman", name: "api-tester" },
        { account: "team", name: "deploy-helper" },
      ].map(({ account, name }) => (
        <div key={`${account}/${name}`} className="flex flex-col items-center gap-1">
          <BlueprintIdentity account={account} name={name} size={64} />
          <span className="text-xs text-muted-foreground">{account}/{name}</span>
        </div>
      ))}
    </div>
  ),
}

export const WithAvatarUrl: Story = {
  name: "With Avatar URL",
  args: {
    account: "acme-corp",
    name: "research-assistant",
    avatarUrl: "https://picsum.photos/128",
    size: 128,
  },
}
