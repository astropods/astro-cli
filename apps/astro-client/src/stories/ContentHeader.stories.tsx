import type { Meta, StoryObj } from "@storybook/react-vite"

import { ContentHeader } from "@/components/ContentHeader"
import { Button } from "@/components/ui/button"

const meta = {
  title: "Design System/Composites/ContentHeader",
  component: ContentHeader,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-2xl border rounded-md">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ContentHeader>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    children: <span className="text-sm font-medium">Page Title</span>,
  },
}

export const WithActions: Story = {
  render: () => (
    <ContentHeader>
      <span className="text-sm font-medium">Deployments</span>
      <div className="ml-auto flex gap-2">
        <Button variant="outline" size="sm">Filter</Button>
        <Button size="sm">New Deployment</Button>
      </div>
    </ContentHeader>
  ),
}

export const WithBreadcrumb: Story = {
  render: () => (
    <ContentHeader>
      <nav className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <span>Agents</span>
        <span>/</span>
        <span className="text-foreground font-medium">my-agent</span>
      </nav>
    </ContentHeader>
  ),
}
