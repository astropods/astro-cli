import type { Meta, StoryObj } from "@storybook/react-vite"

import { BuildUpdateBadge } from "@/components/BuildUpdateBadge"

const meta = {
  title: "Design System/Composites/BuildUpdateBadge",
  component: BuildUpdateBadge,
  parameters: { layout: "padded" },
  argTypes: {
    stacked: { control: "boolean" },
    availableLabel: { control: "boolean" },
  },
} satisfies Meta<typeof BuildUpdateBadge>

export default meta
type Story = StoryObj<typeof meta>

export const UpdateAvailable: Story = {
  args: {
    currentBuildId: "abc123",
    latestBuildId: "def456",
  },
}

export const UpToDate: Story = {
  args: {
    currentBuildId: "abc123",
    latestBuildId: "abc123",
  },
}

export const NoBuild: Story = {
  args: {},
}

export const Stacked: Story = {
  args: {
    currentBuildId: "abc123",
    latestBuildId: "def456",
    stacked: true,
  },
}

export const WithAvailableLabel: Story = {
  args: {
    currentBuildId: "abc123",
    latestBuildId: "def456",
    availableLabel: true,
  },
}

export const StackedWithAvailableLabel: Story = {
  name: "Stacked With Available Label",
  args: {
    currentBuildId: "abc123",
    latestBuildId: "def456",
    stacked: true,
    availableLabel: true,
  },
}
