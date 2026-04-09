import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { InterfacesPicker, type InterfacesPickerProps } from "@/components/deploy/InterfacesPicker";
import { AVAILABLE_ADAPTERS, adapterFields } from "@/components/deploy/useDeployForm";
import type { VariableDisplay } from "@/components/deploy/VariableFields";

const defaultAdapterFieldDefs: Record<string, [string, VariableDisplay][]> = Object.fromEntries(
  AVAILABLE_ADAPTERS.map((a) => [
    a.id,
    adapterFields(a.id).map((f) => [f.key, {
      description: f.description,
      optional: false,
      secret: f.secret,
      label: f.label,
      icon: f.icon,
      defaultValue: f.defaultValue,
      placeholder: f.placeholder,
      helpUrl: f.helpUrl,
      datatype: f.key === "WEB_REQUIRE_AUTH" ? "boolean" : undefined,
    }]),
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
