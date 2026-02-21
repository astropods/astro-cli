import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { InterfacesPicker, type InterfacesPickerProps } from "@/components/deploy/InterfacesPicker";

function InterfacesPickerStateful(props: InterfacesPickerProps) {
  const [selected, setSelected] = useState<string[]>(props.selected);
  const [adapterCreds, setAdapterCreds] = useState<Record<string, string>>(props.adapterCredentials);
  return (
    <InterfacesPicker
      selected={selected}
      onChange={setSelected}
      adapterCredentials={adapterCreds}
      onAdapterCredentialsChange={setAdapterCreds}
    />
  );
}

const meta = {
  title: "Deploy/InterfacesPicker",
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
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const NoneSelected: Story = {
  name: "None Selected",
  args: {
    selected: [],
    onChange: () => {},
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const BothSelected: Story = {
  name: "Both Selected",
  args: {
    selected: ["slack", "web"],
    onChange: () => {},
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};

export const SlackWithCredentials: Story = {
  name: "Slack Selected (Shows Credentials)",
  args: {
    selected: ["slack"],
    onChange: () => {},
    adapterCredentials: {},
    onAdapterCredentialsChange: () => {},
  },
};
