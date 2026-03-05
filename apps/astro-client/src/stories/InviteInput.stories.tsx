import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";

const meta = {
  title: "Components/InviteInput",
  component: InviteInput,
  decorators: [
    (Story) => (
      <div className="max-w-md">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof InviteInput>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

export const WithEntries: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([
      { value: "alice", kind: "account", valid: true },
      { value: "bob", kind: "account", valid: true },
      { value: "carol@example.com", kind: "email", valid: true },
    ]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

export const WithInvalidEmail: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([
      { value: "alice", kind: "account", valid: true },
      { value: "not-an-email", kind: "email", valid: false },
      { value: "bob@example.com", kind: "email", valid: true },
    ]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

export const CustomPlaceholder: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([]);
    return (
      <InviteInput
        entries={entries}
        onChange={setEntries}
        placeholder="Add team members..."
      />
    );
  },
};
