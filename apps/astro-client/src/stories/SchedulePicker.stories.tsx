import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { SchedulePicker } from "@/components/deploy/SchedulePicker"

function SchedulePickerStateful({
  label,
  initialValue = "",
  error,
}: {
  label: string
  initialValue?: string
  error?: string
}) {
  const [value, setValue] = useState(initialValue)
  return <SchedulePicker label={label} value={value} onChange={setValue} error={error} />
}

const meta = {
  title: "Features/Deploy/SchedulePicker",
  component: SchedulePickerStateful,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-md">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SchedulePickerStateful>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    label: "Run Schedule",
  },
}

export const WithPreset: Story = {
  args: {
    label: "Run Schedule",
    initialValue: "0 * * * *",
  },
}

export const WithCustomCron: Story = {
  args: {
    label: "Run Schedule",
    initialValue: "30 9 * * 1-5",
  },
}

export const WithError: Story = {
  args: {
    label: "Run Schedule",
    error: "A schedule is required for cron-triggered agents",
  },
}
