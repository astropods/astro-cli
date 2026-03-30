import type { Meta, StoryObj } from "@storybook/react-vite"

import { PrivacyBadge } from "@/components/PrivacyBadge"

const meta = {
  title: "Design System/Composites/PrivacyBadge",
  component: PrivacyBadge,
  parameters: { layout: "padded" },
} satisfies Meta<typeof PrivacyBadge>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Clickable: Story = {
  args: {
    onClick: () => {},
  },
}
