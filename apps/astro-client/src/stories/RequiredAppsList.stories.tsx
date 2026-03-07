import type { Meta, StoryObj } from "@storybook/react-vite";
import { RequiredAppsList } from "@/components/agent-detail/RequiredAppsList";

const meta = {
  title: "Features/Agents/Sidebar/RequiredAppsList",
  component: RequiredAppsList,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RequiredAppsList>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Single: Story = {
  args: {
    integrations: ["Slack"],
  },
};

export const Multiple: Story = {
  args: {
    integrations: ["Slack", "GitHub", "Linear"],
  },
};

export const UnknownIntegration: Story = {
  args: {
    integrations: ["Slack", "SomeCustomApp"],
  },
};
