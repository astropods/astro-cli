import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { VariableField } from "@/components/deploy/VariableField"
import type { VariableDisplay } from "@/components/deploy/VariableFields"

function VariableFieldStateful({
  fieldKey,
  meta: fieldMeta,
  initialValue = "",
  hasError,
}: {
  fieldKey: string
  meta: VariableDisplay
  initialValue?: string
  hasError?: boolean
}) {
  const [value, setValue] = useState(initialValue)
  return (
    <VariableField
      fieldKey={fieldKey}
      meta={fieldMeta}
      value={value}
      onChange={setValue}
      hasError={hasError}
    />
  )
}

const meta = {
  title: "Features/Deploy/VariableField",
  component: VariableFieldStateful,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-md">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof VariableFieldStateful>

export default meta
type Story = StoryObj<typeof meta>

export const TextInput: Story = {
  args: {
    fieldKey: "API_KEY",
    meta: { description: "Your API key" },
  },
}

export const SecretField: Story = {
  args: {
    fieldKey: "SLACK_BOT_TOKEN",
    meta: { description: "Slack bot token", secret: true },
    initialValue: "xoxb-1234567890",
  },
}

export const SelectDropdown: Story = {
  args: {
    fieldKey: "MODEL",
    meta: {
      description: "Model to use",
      displayAs: "select",
      options: ["gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"],
    },
  },
}

export const BooleanToggle: Story = {
  args: {
    fieldKey: "WEB_REQUIRE_AUTH",
    meta: {
      label: "Require authentication",
      description: "Restrict access to signed-in users only",
      icon: "shield",
      datatype: "boolean",
    },
    initialValue: "true",
  },
}

export const NumberField: Story = {
  args: {
    fieldKey: "MAX_RETRIES",
    meta: { description: "Maximum retry attempts", datatype: "number" },
    initialValue: "3",
  },
}

export const TextareaField: Story = {
  args: {
    fieldKey: "SYSTEM_PROMPT",
    meta: { description: "System prompt for the agent", displayAs: "long-text" },
  },
}

export const ArrayField: Story = {
  args: {
    fieldKey: "ALLOWED_DOMAINS",
    meta: { description: "Allowed domains", datatype: "array" },
  },
}

export const WithError: Story = {
  args: {
    fieldKey: "DATABASE_URL",
    meta: { description: "Database connection string" },
    hasError: true,
  },
}

export const WithPlaceholder: Story = {
  args: {
    fieldKey: "WEBHOOK_URL",
    meta: { description: "Webhook endpoint", placeholder: "https://hooks.example.com/..." },
  },
}
