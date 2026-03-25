import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { BlueprintDetailBreadcrumb } from "@/components/blueprint-detail/BlueprintDetailBreadcrumb";

const meta = {
  title: "Features/Agents/BlueprintDetailBreadcrumb",
  component: BlueprintDetailBreadcrumb,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof BlueprintDetailBreadcrumb>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    account: "sohumonlocal",
    blueprintName: "api-changelog-writer",
  },
};
