import { useState } from "react";
import { http, HttpResponse, delay } from "msw";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import type { AccountSearchResponse } from "@/lib/api";

const meta = {
  title: "Design System/Composites/InviteInput",
  decorators: [
    (Story) => (
      <div className="max-w-md">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

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
      { value: "alice", kind: "account", valid: true, displayName: "Alice Chen" },
      { value: "bob", kind: "account", valid: true, displayName: "Bob Martinez" },
      { value: "carol@example.com", kind: "email", valid: true },
    ]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

export const WithInvalidEmail: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([
      { value: "alice", kind: "account", valid: true, displayName: "Alice Chen" },
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

/** Type 3+ chars (e.g. "ali") — dropdown should show alice/alicia/etc. */
export const SearchAccounts: Story = {
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

/** Slow API — type 3+ chars to see in-flight state. */
export const SearchLoading: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/v1/accounts/search", async () => {
          await delay("infinite");
          return HttpResponse.json<AccountSearchResponse>({ results: [] });
        }),
      ],
    },
  },
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};

/** Empty API — type 3+ chars; dropdown stays hidden (no matches). */
export const SearchNoResults: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/v1/accounts/search", () =>
          HttpResponse.json<AccountSearchResponse>({ results: [] }),
        ),
      ],
    },
  },
  render: () => {
    const [entries, setEntries] = useState<InviteEntry[]>([]);
    return <InviteInput entries={entries} onChange={setEntries} />;
  },
};
