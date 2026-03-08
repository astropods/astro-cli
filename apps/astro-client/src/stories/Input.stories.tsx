import type { Meta, StoryObj } from "@storybook/react-vite"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

const meta = {
  title: "Design System/Primitives/Input",
  component: Input,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "code"],
    },
    disabled: { control: "boolean" },
    placeholder: { control: "text" },
  },
} satisfies Meta<typeof Input>

export default meta
type Story = StoryObj<typeof meta>

export const AllStates: Story = {
  render: () => (
    <div className="flex flex-col gap-8 max-w-md">
      <div>
        <p className="text-body-sm font-semibold text-foreground mb-3">Default</p>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Input placeholder="Display name" />
            <span className="text-mono-sm font-mono text-muted-foreground">Empty</span>
          </div>
          <div className="flex flex-col gap-1">
            <Input defaultValue="Taylor Green" />
            <span className="text-mono-sm font-mono text-muted-foreground">Filled</span>
          </div>
          <div className="flex flex-col gap-1">
            <Input defaultValue="Taylor Green" disabled />
            <span className="text-mono-sm font-mono text-muted-foreground">Disabled</span>
          </div>
          <div className="flex flex-col gap-1">
            <Input defaultValue="bad-email" aria-invalid="true" />
            <span className="text-mono-sm font-mono text-muted-foreground">Invalid</span>
          </div>
        </div>
      </div>

      <div>
        <p className="text-body-sm font-semibold text-foreground mb-3">Code variant</p>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Input variant="code" placeholder="API key or environment variable" />
            <span className="text-mono-sm font-mono text-muted-foreground">Empty</span>
          </div>
          <div className="flex flex-col gap-1">
            <Input variant="code" defaultValue="sk-ant-api03-..." />
            <span className="text-mono-sm font-mono text-muted-foreground">Filled</span>
          </div>
        </div>
      </div>
    </div>
  ),
}

export const Default: Story = {
  args: {
    placeholder: "Display name",
  },
}

export const Filled: Story = {
  args: {
    defaultValue: "Taylor Green",
  },
}

export const Code: Story = {
  args: {
    variant: "code",
    defaultValue: "sk-ant-api03-...",
  },
}

export const Disabled: Story = {
  args: {
    defaultValue: "Taylor Green",
    disabled: true,
  },
}

export const Invalid: Story = {
  args: {
    defaultValue: "bad-email",
    "aria-invalid": "true",
  },
}

export const WithLabel: Story = {
  render: () => (
    <div className="flex flex-col gap-1.5 max-w-xs">
      <Label htmlFor="name">Display Name</Label>
      <Input id="name" defaultValue="Taylor Green" />
    </div>
  ),
}

export const WithLabelCode: Story = {
  render: () => (
    <div className="flex flex-col gap-1.5 max-w-xs">
      <Label htmlFor="key">API Key</Label>
      <Input id="key" variant="code" defaultValue="sk-ant-api03-..." />
    </div>
  ),
}
