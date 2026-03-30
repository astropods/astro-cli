import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { Button } from "@/components/ui/button"
import { TradingCardModal } from "@/components/trading-card/TradingCardModal"
import type { CardData } from "astro-trading-card"

const mockCardData: CardData = {
  name: "research-assistant",
  displayName: "Research Assistant",
  account: "acme-corp",
  description: "An AI agent that helps with research tasks, summarizing papers, and finding relevant information.",
  tags: ["research", "productivity"],
  heartCount: 42,
  stats: [
    { label: "Deployments", value: "128" },
    { label: "Uptime", value: "99.9%" },
  ],
  barcodeId: "agent-ra-001",
}

const meta = {
  title: "Features/TradingCard/TradingCardModal",
  parameters: { layout: "centered" },
} satisfies Meta

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <>
        <Button onClick={() => setOpen(true)}>Show Trading Card</Button>
        <TradingCardModal
          open={open}
          onOpenChange={setOpen}
          data={mockCardData}
        />
      </>
    )
  },
}

export const WithMinimalData: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <>
        <Button variant="outline" onClick={() => setOpen(true)}>Show Minimal Card</Button>
        <TradingCardModal
          open={open}
          onOpenChange={setOpen}
          data={{
            name: "slack-bot",
            account: "postman",
          }}
        />
      </>
    )
  },
}
