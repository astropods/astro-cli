import type { Meta, StoryObj } from "@storybook/react-vite";

import { PageTitle } from "@/components/PageTitle";
import { Button } from "@/components/ui/button";
import { PaperAirplaneIcon } from "@heroicons/react/24/outline";

const meta = {
  title: "Design System/Composites/PageTitle",
  component: PageTitle,
} satisfies Meta<typeof PageTitle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Available Agents",
    subtitle: "Browse agents available within your organization",
  },
};

export const WithAction: Story = {
  name: "With Action",
  args: {
    title: "Available Agents",
    subtitle: "Browse agents available within your organization",
    actions: (
      <Button size="default">
        <PaperAirplaneIcon className="size-4" />
        Request agent
      </Button>
    ),
  },
};

export const TitleOnly: Story = {
  name: "Title Only",
  args: {
    title: "Settings",
  },
};
