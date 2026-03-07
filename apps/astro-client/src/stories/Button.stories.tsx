import type { Meta, StoryObj } from "@storybook/react-vite"
import { PlusIcon, EnvelopeIcon, ArrowPathIcon } from "@heroicons/react/24/outline"

import { Button } from "@/components/ui/button"

const meta = {
  title: "Design System/Primitives/Button",
  component: Button,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "outline", "destructive", "ghost", "link"],
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

const variants = ["default", "outline", "destructive", "ghost", "link"] as const
const textSizes = ["xs", "sm", "default", "lg"] as const
const iconSizes = ["icon-xs", "icon-sm", "icon", "icon-lg"] as const

export const AllVariantsAndSizes: Story = {
  render: () => (
    <div className="space-y-6">
      <table className="border-separate border-spacing-2">
        <thead>
          <tr>
            <th className="text-left text-xs font-medium text-muted-foreground pr-4" />
            {textSizes.map((s) => (
              <th key={s} className="text-center text-xs font-medium text-muted-foreground px-2">{s}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {variants.map((v) => (
            <tr key={v}>
              <td className="text-xs font-medium text-muted-foreground pr-4 capitalize">{v}</td>
              {textSizes.map((s) => (
                <td key={s} className="text-center px-2">
                  <Button variant={v} size={s}>Button</Button>
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <div>
        <p className="text-xs font-medium text-muted-foreground mb-2">Icon sizes</p>
        <div className="flex items-end gap-3">
          {iconSizes.map((s) => (
            <div key={s} className="flex flex-col items-center gap-1">
              <Button variant="outline" size={s}><PlusIcon className="size-4" /></Button>
              <span className="text-[10px] text-muted-foreground">{s}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  ),
}

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

export const AllDisabled: Story = {
  render: () => (
    <div className="flex items-end gap-3">
      {variants.map((v) => (
        <Button key={v} variant={v} disabled>
          {v.charAt(0).toUpperCase() + v.slice(1)}
        </Button>
      ))}
    </div>
  ),
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
