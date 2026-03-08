import type { Meta, StoryObj } from "@storybook/react-vite"

import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"

const meta = {
  title: "Design System/Primitives/Textarea",
  component: Textarea,
  argTypes: {
    disabled: { control: "boolean" },
    placeholder: { control: "text" },
  },
} satisfies Meta<typeof Textarea>

export default meta
type Story = StoryObj<typeof meta>

export const AllStates: Story = {
  render: () => (
    <div className="flex flex-col gap-4 max-w-md">
      <div className="flex flex-col gap-1">
        <Textarea placeholder="Enter a description..." />
        <span className="text-mono-sm font-mono text-muted-foreground">Empty</span>
      </div>
      <div className="flex flex-col gap-1">
        <Textarea defaultValue="This agent summarizes Slack threads, surfaces action items, and drafts replies across channels and DMs." />
        <span className="text-mono-sm font-mono text-muted-foreground">Filled</span>
      </div>
      <div className="flex flex-col gap-1">
        <Textarea defaultValue="Locked content" disabled />
        <span className="text-mono-sm font-mono text-muted-foreground">Disabled</span>
      </div>
      <div className="flex flex-col gap-1">
        <Textarea defaultValue="x" aria-invalid="true" />
        <span className="text-mono-sm font-mono text-muted-foreground">Invalid</span>
      </div>
    </div>
  ),
}

export const Default: Story = {
  args: {
    placeholder: "Enter a description...",
  },
}

export const Filled: Story = {
  args: {
    defaultValue: "This agent summarizes Slack threads, surfaces action items, and drafts replies across channels and DMs.",
  },
}

export const Disabled: Story = {
  args: {
    defaultValue: "Locked content",
    disabled: true,
  },
}

export const Invalid: Story = {
  args: {
    defaultValue: "x",
    "aria-invalid": "true",
  },
}

export const WithLabel: Story = {
  render: () => (
    <div className="flex flex-col gap-1.5 max-w-md">
      <Label htmlFor="desc">Description</Label>
      <Textarea id="desc" placeholder="Enter a description..." />
    </div>
  ),
}
