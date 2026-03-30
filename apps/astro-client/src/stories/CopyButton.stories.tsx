import type { Meta, StoryObj } from "@storybook/react-vite"

import { CopyButton } from "@/components/ui/copy-button"

const meta = {
  title: "Design System/Primitives/CopyButton",
  component: CopyButton,
  parameters: { layout: "padded" },
  argTypes: {
    size: { control: "number" },
  },
} satisfies Meta<typeof CopyButton>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    copyText: "Hello, world!",
  },
}

export const WithTitle: Story = {
  name: "With Title",
  args: {
    copyText: "sk-1234567890abcdef",
    title: "Copy API key",
  },
}

export const LargeSize: Story = {
  name: "Large Size",
  args: {
    copyText: "Larger icon",
    size: 18,
  },
}

export const InContext: Story = {
  name: "In Context",
  args: {
    copyText: "astro deploy my-agent",
  },
  render: () => (
    <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
      <code className="flex-1 font-mono text-xs">astro deploy my-agent</code>
      <CopyButton copyText="astro deploy my-agent" title="Copy command" />
    </div>
  ),
}
