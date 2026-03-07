import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { InterfacesPicker, type InterfacesPickerProps } from "@/components/deploy/InterfacesPicker";
import { ADAPTER_CREDENTIALS } from "@/components/deploy/useDeployForm";
import type { VariableDisplay } from "@/components/deploy/VariableFields";

// Default cred defs used in stories (mirrors what useDeployForm computes from a template)
const defaultAdapterCredDefs: Record<string, [string, VariableDisplay][]> = Object.fromEntries(
  Object.entries(ADAPTER_CREDENTIALS).map(([id, creds]) => [
    id,
    creds.map((c) => [c.key, { description: c.description, optional: false, secret: c.secret, label: c.label, placeholder: c.placeholder, helpUrl: c.helpUrl }]),
  ]),
);

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
    adapterCredDefs: defaultAdapterCredDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const NoneSelected: Story = {
  name: "None Selected",
  args: {
    selected: [],
    onChange: () => {},
    adapterCredDefs: defaultAdapterCredDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const BothSelected: Story = {
  name: "Both Selected",
  args: {
    selected: ["slack", "web"],
    onChange: () => {},
    adapterCredDefs: defaultAdapterCredDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const SlackWithCredentials: Story = {
  name: "Slack Selected (Shows Credentials)",
  args: {
    selected: ["slack"],
    onChange: () => {},
    adapterCredDefs: defaultAdapterCredDefs,
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};
