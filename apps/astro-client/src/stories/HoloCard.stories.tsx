import type { Meta, StoryObj } from "@storybook/react-vite"

import { HoloCard } from "@/components/trading-card/HoloCard"

const meta = {
  title: "Features/TradingCard/HoloCard",
  parameters: { layout: "centered" },
} satisfies Meta

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => (
    <HoloCard>
      <div
        className="flex items-center justify-center rounded-2xl"
        style={{
          width: 350,
          height: 560,
          background: "linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)",
        }}
      >
        <div className="text-center text-white">
          <div className="text-4xl mb-2">🤖</div>
          <div className="text-lg font-bold">Research Assistant</div>
          <div className="text-sm opacity-60 mt-1">acme-corp</div>
        </div>
      </div>
    </HoloCard>
  ),
}

export const WithCustomContent: Story = {
  render: () => (
    <HoloCard>
      <div
        className="flex flex-col items-center justify-center gap-4 rounded-2xl p-8"
        style={{
          width: 300,
          height: 420,
          background: "linear-gradient(135deg, #0d1b2a 0%, #1b263b 50%, #415a77 100%)",
        }}
      >
        <div className="size-20 rounded-full bg-white/10 flex items-center justify-center text-3xl">
          ⚡
        </div>
        <div className="text-center text-white">
          <div className="font-bold text-lg">Deploy Helper</div>
          <div className="text-xs opacity-50 mt-1 uppercase tracking-widest">Agent Card</div>
        </div>
      </div>
    </HoloCard>
  ),
}
