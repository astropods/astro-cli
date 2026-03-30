import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { AccountPicker } from "@/components/deploy/AccountPicker"
import type { Account } from "@/lib/api"

const personalAccount: Account = {
  id: "1",
  name: "johndoe",
  type: "personal",
}

const orgAccounts: Account[] = [
  { id: "2", name: "acme-corp", type: "organization" },
  { id: "3", name: "postman-labs", type: "organization" },
  { id: "4", name: "dev-team", type: "organization" },
]

function AccountPickerStateful({ accounts }: { accounts: Account[] }) {
  const [selected, setSelected] = useState(accounts[0]?.name ?? "")
  return <AccountPicker accounts={accounts} selected={selected} onChange={setSelected} />
}

const meta = {
  title: "Features/Deploy/AccountPicker",
  component: AccountPickerStateful,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-sm">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof AccountPickerStateful>

export default meta
type Story = StoryObj<typeof meta>

export const SinglePersonal: Story = {
  args: {
    accounts: [personalAccount],
  },
}

export const WithOrganizations: Story = {
  args: {
    accounts: [personalAccount, ...orgAccounts],
  },
}

export const ManyAccounts: Story = {
  args: {
    accounts: [
      personalAccount,
      ...orgAccounts,
      { id: "5", name: "frontend-team", type: "organization" },
      { id: "6", name: "backend-team", type: "organization" },
      { id: "7", name: "data-science", type: "organization" },
    ],
  },
}
