import type { Meta, StoryObj } from "@storybook/react-vite"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Label } from "@/components/ui/label"

function SelectDemo({
  placeholder = "Select a model",
  disabled = false,
  defaultValue,
}: {
  placeholder?: string
  disabled?: boolean
  defaultValue?: string
}) {
  return (
    <Select disabled={disabled} defaultValue={defaultValue}>
      <SelectTrigger className="w-[280px]">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="claude-opus">Claude Opus 4.6</SelectItem>
        <SelectItem value="claude-sonnet">Claude Sonnet 4.6</SelectItem>
        <SelectItem value="claude-haiku">Claude Haiku 4.5</SelectItem>
        <SelectItem value="gpt-4o">GPT-4o</SelectItem>
      </SelectContent>
    </Select>
  )
}

const meta = {
  title: "Design System/Primitives/Select",
  component: SelectDemo,
} satisfies Meta<typeof SelectDemo>

export default meta
type Story = StoryObj<typeof meta>

export const AllStates: Story = {
  render: () => (
    <div className="flex flex-col gap-4 max-w-sm">
      <div className="flex flex-col gap-1">
        <SelectDemo />
        <span className="text-mono-sm font-mono text-muted-foreground">Empty</span>
      </div>
      <div className="flex flex-col gap-1">
        <SelectDemo defaultValue="claude-sonnet" />
        <span className="text-mono-sm font-mono text-muted-foreground">Filled</span>
      </div>
      <div className="flex flex-col gap-1">
        <SelectDemo defaultValue="claude-sonnet" disabled />
        <span className="text-mono-sm font-mono text-muted-foreground">Disabled</span>
      </div>
    </div>
  ),
}

export const Default: Story = {
  args: {
    placeholder: "Select a model",
  },
}

export const WithValue: Story = {
  args: {
    defaultValue: "claude-sonnet",
  },
}

export const Disabled: Story = {
  args: {
    defaultValue: "claude-sonnet",
    disabled: true,
  },
}

export const WithLabel: Story = {
  render: () => (
    <div className="flex flex-col gap-1.5 max-w-sm">
      <Label htmlFor="model">Model</Label>
      <SelectDemo />
    </div>
  ),
}
