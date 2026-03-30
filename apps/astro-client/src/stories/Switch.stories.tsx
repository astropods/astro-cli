import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"

const meta = {
  title: "Design System/Primitives/Switch",
  component: Switch,
  argTypes: {
    disabled: { control: "boolean" },
  },
} satisfies Meta<typeof Switch>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => {
    const [checked, setChecked] = useState(false)
    return <Switch checked={checked} onCheckedChange={setChecked} />
  },
}

export const Checked: Story = {
  render: () => {
    const [checked, setChecked] = useState(true)
    return <Switch checked={checked} onCheckedChange={setChecked} />
  },
}

export const Disabled: Story = {
  args: {
    disabled: true,
  },
}

export const DisabledChecked: Story = {
  args: {
    disabled: true,
    checked: true,
  },
}

export const WithLabel: Story = {
  render: () => {
    const [checked, setChecked] = useState(false)
    return (
      <div className="flex items-center gap-2">
        <Switch id="notifications" checked={checked} onCheckedChange={setChecked} />
        <Label htmlFor="notifications">Enable notifications</Label>
      </div>
    )
  },
}
