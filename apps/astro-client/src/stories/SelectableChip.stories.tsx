import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { Check } from "lucide-react"

import { SelectableChip } from "@/components/ui/SelectableChip"

const meta = {
  title: "Design System/Primitives/SelectableChip",
  component: SelectableChip,
  argTypes: {
    tone: {
      control: "select",
      options: ["primary", "success", "destructive"],
    },
    selected: { control: "boolean" },
    disabled: { control: "boolean" },
  },
  args: {
    tone: "primary",
    selected: false,
    children: "Correct info",
  },
} satisfies Meta<typeof SelectableChip>

export default meta
type Story = StoryObj<typeof meta>

const tones = ["primary", "success", "destructive"] as const

export const Default: Story = {}

export const Selected: Story = {
  args: {
    selected: true,
    tone: "success",
    children: (
      <>
        Correct info <Check aria-hidden className="size-4" />
      </>
    ),
  },
}

export const AllTones: Story = {
  render: () => (
    <table className="border-separate border-spacing-3">
      <thead>
        <tr>
          <th className="pr-4 text-left text-xs font-medium text-muted-foreground" />
          <th className="px-2 text-xs font-medium text-muted-foreground">Unselected</th>
          <th className="px-2 text-xs font-medium text-muted-foreground">Selected</th>
        </tr>
      </thead>
      <tbody>
        {tones.map((tone) => (
          <tr key={tone}>
            <td className="pr-4 text-xs font-medium capitalize text-muted-foreground">
              {tone}
            </td>
            <td className="px-2">
              <SelectableChip tone={tone} selected={false}>
                Followed instruction
              </SelectableChip>
            </td>
            <td className="px-2">
              <SelectableChip tone={tone} selected>
                Followed instruction <Check aria-hidden className="size-4" />
              </SelectableChip>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  ),
}

const CRITERIA = [
  "Correct info",
  "Complete",
  "Followed instruction",
  "Clear & well-scoped",
  "Appropriate tone",
]

export const MultiSelectGroup: Story = {
  render: () => {
    const [selected, setSelected] = useState<Set<string>>(() => new Set(["Correct info"]))
    const toggle = (label: string) =>
      setSelected((prev) => {
        const next = new Set(prev)
        if (next.has(label)) {
          next.delete(label)
        } else {
          next.add(label)
        }
        return next
      })

    return (
      <div className="flex max-w-lg flex-wrap gap-2">
        {CRITERIA.map((label) => {
          const isSelected = selected.has(label)
          return (
            <SelectableChip
              key={label}
              tone="success"
              selected={isSelected}
              onClick={() => toggle(label)}
            >
              {label}
              {isSelected && <Check aria-hidden className="size-4" />}
            </SelectableChip>
          )
        })}
      </div>
    )
  },
}

export const Disabled: Story = {
  args: {
    disabled: true,
    children: "Correct info",
  },
}
