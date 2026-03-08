import type { Meta, StoryObj } from "@storybook/react-vite"

import { Label } from "@/components/ui/label"

const meta = {
  title: "Design System/Primitives/Label",
  component: Label,
  argTypes: {
    children: { control: "text" },
  },
} satisfies Meta<typeof Label>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    children: "Display Name",
  },
}

export const Examples: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {["Display Name", "API Key", "Description", "Status"].map((text) => (
        <Label key={text}>{text}</Label>
      ))}
    </div>
  ),
}
