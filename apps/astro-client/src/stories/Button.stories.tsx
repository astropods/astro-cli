import type { Meta, StoryObj } from "@storybook/react-vite"
import { PlusIcon, EnvelopeIcon, ArrowPathIcon } from "@heroicons/react/24/outline"

import { Button } from "@/components/ui/button"

const meta = {
  title: "Components/Button",
  component: Button,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "outline", "destructive", "secondary", "ghost", "link"],
    },
    size: {
      control: "select",
      options: ["default", "xs", "sm", "lg", "icon", "icon-xs", "icon-sm", "icon-lg"],
    },
    disabled: { control: "boolean" },
    asChild: { table: { disable: true } },
  },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    children: "Button",
  },
}

export const Outline: Story = {
  args: {
    variant: "outline",
    children: "Button",
  },
}

export const Destructive: Story = {
  args: {
    variant: "destructive",
    children: "Button",
  },
}

export const Secondary: Story = {
  args: {
    variant: "secondary",
    children: "Button",
  },
}

export const Ghost: Story = {
  args: {
    variant: "ghost",
    children: "Button",
  },
}

export const Link: Story = {
  args: {
    variant: "link",
    children: "Button",
  },
}

export const WithIcon: Story = {
  args: {
    children: (
      <>
        <EnvelopeIcon className="size-4" /> Send Email
      </>
    ),
  },
}

export const IconButton: Story = {
  args: {
    variant: "outline",
    size: "icon",
    children: <PlusIcon className="size-4" />,
  },
}

export const Loading: Story = {
  args: {
    disabled: true,
    children: (
      <>
        <ArrowPathIcon className="size-4 animate-spin" /> Please wait
      </>
    ),
  },
}

export const Disabled: Story = {
  args: {
    disabled: true,
    children: "Button",
  },
}

export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <Button size="xs">Extra Small</Button>
      <Button size="sm">Small</Button>
      <Button>Default</Button>
      <Button size="lg">Large</Button>
    </div>
  ),
}

export const IconSizes: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      <Button variant="outline" size="icon-xs"><PlusIcon className="size-4" /></Button>
      <Button variant="outline" size="icon-sm"><PlusIcon className="size-4" /></Button>
      <Button variant="outline" size="icon"><PlusIcon className="size-4" /></Button>
      <Button variant="outline" size="icon-lg"><PlusIcon className="size-4" /></Button>
    </div>
  ),
}
