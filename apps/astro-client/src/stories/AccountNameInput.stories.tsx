import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { AccountNameInput } from "@/components/AccountNameInput"

function AccountNameInputStateful(props: {
  initialValue?: string
  placeholder?: string
  isChecking: boolean
  isAvailable: boolean
  displayError: string | null
}) {
  const [value, setValue] = useState(props.initialValue ?? "")
  return (
    <AccountNameInput
      value={value}
      onChange={setValue}
      placeholder={props.placeholder}
      isChecking={props.isChecking}
      isAvailable={props.isAvailable}
      displayError={props.displayError}
    />
  )
}

const meta = {
  title: "Design System/Composites/AccountNameInput",
  component: AccountNameInputStateful,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-sm">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof AccountNameInputStateful>

export default meta
type Story = StoryObj<typeof meta>

export const Empty: Story = {
  args: {
    isChecking: false,
    isAvailable: false,
    displayError: null,
  },
}

export const Checking: Story = {
  args: {
    initialValue: "my-agent",
    isChecking: true,
    isAvailable: false,
    displayError: null,
  },
}

export const Available: Story = {
  args: {
    initialValue: "my-agent",
    isChecking: false,
    isAvailable: true,
    displayError: null,
  },
}

export const Unavailable: Story = {
  args: {
    initialValue: "taken-name",
    isChecking: false,
    isAvailable: false,
    displayError: "This username is already taken",
  },
}

export const WithCustomPlaceholder: Story = {
  args: {
    isChecking: false,
    isAvailable: false,
    displayError: null,
    placeholder: "organization-name",
  },
}
