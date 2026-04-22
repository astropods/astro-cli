import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { InterfacesPicker, type InterfacesPickerProps } from "@/components/deploy/InterfacesPicker";

import type { VariableDisplay } from "@/components/deploy/VariableFields";

const defaultAdapterFieldDefs: Record<string, [string, VariableDisplay][]> = {
  slack: [
    ["SLACK_BOT_TOKEN", { label: "Slack Bot Token", description: "Slack bot token for API access", secret: true, placeholder: "xoxb-..." }],
    ["SLACK_APP_TOKEN", { label: "Slack App Token", description: "Slack app token for socket mode", secret: true, placeholder: "xapp-..." }],
    ["SLACK_ACTIONABLE_REACTIONS", { label: "Actionable Reactions", description: "Emoji names the bot acts on", optional: true, placeholder: "ticket, bug" }],
    ["SLACK_ALLOWED_CHANNEL_IDS", { label: "Allowed Channel IDs", description: "Restrict to specific channels", optional: true, placeholder: "C12345, C67890" }],
    ["SLACK_ALLOWED_USER_IDS", { label: "Allowed User IDs", description: "Restrict to specific users", optional: true, placeholder: "U12345, U67890" }],
  ],
  web: [[
    "WEB_REQUIRE_AUTH",
    {
      label: "Require authentication",
      description: "Restrict access to signed-in users only",
      icon: "shield",
      datatype: "boolean",
      optional: false,
    },
  ]],
};

function InterfacesPickerStateful(props: InterfacesPickerProps) {
  const [selected, setSelected] = useState<string[]>(props.selected);
  const [adapterCreds, setAdapterCreds] = useState<Record<string, string>>(props.adapterCredentials);
  return (
    <InterfacesPicker
      {...props}
      selected={selected}
      onChange={setSelected}
      adapterCredentials={adapterCreds}
      onAdapterCredentialsChange={setAdapterCreds}
    />
  );
}

const meta = {
  title: "Features/Deploy/InterfacesPicker",
  component: InterfacesPickerStateful,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof InterfacesPickerStateful>;

export default meta;
type Story = StoryObj<typeof meta>;

export const WebSelected: Story = {
  name: "Web Selected (Default)",
  args: {
    selected: ["web"],
    onChange: () => {},
    adapterCredDefs: defaultAdapterFieldDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
    credentialLayoutByAdapter: { web: "inline-card", slack: "inline-card" },
  },
};

export const NoneSelected: Story = {
  name: "None Selected",
  args: {
    selected: [],
    onChange: () => {},
    adapterCredDefs: defaultAdapterFieldDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const BothSelected: Story = {
  name: "Both Selected",
  args: {
    selected: ["slack", "web"],
    onChange: () => {},
    adapterCredDefs: defaultAdapterFieldDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
    credentialLayoutByAdapter: { web: "inline-card", slack: "inline-card" },
  },
};

export const SlackWithCredentials: Story = {
  name: "Slack Selected (Shows Credentials)",
  args: {
    selected: ["slack"],
    onChange: () => {},
    adapterCredDefs: defaultAdapterFieldDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
    credentialLayoutByAdapter: { web: "inline-card", slack: "inline-card" },
  },
};
