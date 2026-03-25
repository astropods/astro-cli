import type { Meta, StoryObj } from "@storybook/react-vite";
import { RequiredAppsList } from "@/components/blueprint-detail/RequiredAppsList";
import type { ResolvedIntegration } from "@/lib/api";

const ri = (id: string, name: string): ResolvedIntegration => ({ id, name, known: true });

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
    integrations: [ri("slack", "Slack")],
  },
};

export const Multiple: Story = {
  args: {
    integrations: [ri("slack", "Slack"), ri("github", "GitHub"), ri("linear", "Linear")],
  },
};

export const UnknownIntegration: Story = {
  args: {
    integrations: [ri("slack", "Slack"), ri("some-custom-app", "SomeCustomApp")],
  },
};
