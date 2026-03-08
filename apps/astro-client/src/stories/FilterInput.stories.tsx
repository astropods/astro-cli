import type { Meta, StoryObj } from "@storybook/react-vite"

import { FilterInput } from "@/components/FilterInput"

const meta = {
  title: "Design System/Primitives/FilterInput",
  component: FilterInput,
  argTypes: {
    disabled: { control: "boolean" },
    placeholder: { control: "text" },
  },
} satisfies Meta<typeof FilterInput>

export default meta
type Story = StoryObj<typeof meta>

export const AllStates: Story = {
  render: () => (
    <div className="flex flex-col gap-4 max-w-sm">
      <div className="flex flex-col gap-1">
        <FilterInput placeholder="Filter agents..." />
        <span className="text-mono-sm font-mono text-muted-foreground">Empty</span>
      </div>
      <div className="flex flex-col gap-1">
        <FilterInput defaultValue="slack" />
        <span className="text-mono-sm font-mono text-muted-foreground">Filled</span>
      </div>
      <div className="flex flex-col gap-1">
        <FilterInput defaultValue="slack" disabled />
        <span className="text-mono-sm font-mono text-muted-foreground">Disabled</span>
      </div>
    </div>
  ),
}

export const Default: Story = {
  args: {
    placeholder: "Filter agents...",
  },
}

export const Filled: Story = {
  args: {
    defaultValue: "slack",
  },
}

export const Disabled: Story = {
  args: {
    defaultValue: "slack",
    disabled: true,
  },
}
