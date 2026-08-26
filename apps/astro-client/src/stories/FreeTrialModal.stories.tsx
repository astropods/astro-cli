import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { Button } from "@/components/ui/button"
import { FreeTrialModal } from "@/components/FreeTrialModal"

const meta = {
  title: "Design System/Composites/FreeTrialModal",
  parameters: { layout: "centered" },
} satisfies Meta

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <>
        <Button onClick={() => setOpen(true)}>Show free trial modal</Button>
        <FreeTrialModal
          open={open}
          onOpenChange={setOpen}
          credits={20}
          onCta={() => setOpen(false)}
        />
      </>
    )
  },
}

export const LargerGrant: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <>
        <Button onClick={() => setOpen(true)}>Show $100 grant</Button>
        <FreeTrialModal
          open={open}
          onOpenChange={setOpen}
          credits={100}
          ctaLabel="Start building"
          onCta={() => setOpen(false)}
        />
      </>
    )
  },
}
