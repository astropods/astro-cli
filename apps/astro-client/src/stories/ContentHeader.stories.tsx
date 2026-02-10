import type { Meta, StoryObj } from "@storybook/react-vite";

import { ContentHeader } from "@/components/ContentHeader";

const meta = {
  title: "Layout/ContentHeader",
  component: ContentHeader,
} satisfies Meta<typeof ContentHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};

export const WithChildren: Story = {
  name: "With Children",
  render: () => (
    <ContentHeader>
      <span className="text-sm text-muted-foreground">
        Custom header content goes here
      </span>
    </ContentHeader>
  ),
};
