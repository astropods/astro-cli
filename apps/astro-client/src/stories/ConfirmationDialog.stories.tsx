import { useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"

import { Button } from "@/components/ui/button"
import { ConfirmationDialog } from "@/components/ConfirmationDialog"

const meta = {
  title: "Design System/Composites/ConfirmationDialog",
} satisfies Meta

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <ConfirmationDialog
        open={open}
        onOpenChange={setOpen}
        title="Delete Deployment"
        description="This will permanently delete the deployment and all associated data. This action cannot be undone."
        checkboxLabel="I understand this action is irreversible"
        actionLabel="Delete"
        pendingLabel="Deleting..."
        defaultErrorMessage="Failed to delete deployment"
        isPending={false}
        canConfirm={true}
        onConfirm={() => setOpen(false)}
        trigger={<Button variant="destructive">Delete Deployment</Button>}
      />
    )
  },
}

export const WithError: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <ConfirmationDialog
        open={open}
        onOpenChange={setOpen}
        title="Delete Account"
        description="This will permanently delete your account."
        checkboxLabel="I understand this cannot be undone"
        actionLabel="Delete Account"
        pendingLabel="Deleting..."
        defaultErrorMessage="Something went wrong"
        error={new Error("Insufficient permissions to delete this account")}
        isPending={false}
        canConfirm={true}
        onConfirm={() => {}}
        trigger={<Button variant="destructive">Delete Account</Button>}
      />
    )
  },
}

export const Pending: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <ConfirmationDialog
        open={open}
        onOpenChange={setOpen}
        title="Archive Agent"
        description="This will archive the agent and stop all running deployments."
        checkboxLabel="I want to archive this agent"
        actionLabel="Archive"
        pendingLabel="Archiving..."
        defaultErrorMessage="Failed to archive"
        isPending={true}
        canConfirm={true}
        onConfirm={() => {}}
        trigger={<Button variant="outline">Archive Agent</Button>}
      />
    )
  },
}

export const WithCustomChildren: Story = {
  render: () => {
    const [open, setOpen] = useState(false)
    return (
      <ConfirmationDialog
        open={open}
        onOpenChange={setOpen}
        title="Transfer Ownership"
        description="Transfer this agent to another account."
        checkboxLabel="I confirm I want to transfer ownership"
        actionLabel="Transfer"
        pendingLabel="Transferring..."
        defaultErrorMessage="Transfer failed"
        isPending={false}
        canConfirm={true}
        onConfirm={() => setOpen(false)}
        trigger={<Button variant="outline">Transfer</Button>}
      >
        <div className="rounded-md border p-3 text-sm text-muted-foreground">
          The new owner will have full control over this agent and its deployments.
        </div>
      </ConfirmationDialog>
    )
  },
}
