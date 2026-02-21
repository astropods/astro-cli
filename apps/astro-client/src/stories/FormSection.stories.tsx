import type { Meta, StoryObj } from "@storybook/react-vite";

import { FormSection } from "@/components/deploy/FormSection";

const meta = {
  title: "Deploy/FormSection",
  component: FormSection,
} satisfies Meta<typeof FormSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Configuration",
    description: "Required configuration for this agent.",
    children: (
      <div className="rounded-[6px] bg-stone-100 p-4 text-sm text-muted-foreground">
        Form content goes here
      </div>
    ),
  },
};

export const Messaging: Story = {
  args: {
    title: "Messaging",
    description: "Choose how you want to interact with the agent.",
    children: (
      <div className="rounded-[6px] bg-stone-100 p-4 text-sm text-muted-foreground">
        Interface picker would go here
      </div>
    ),
  },
};

export const Optional: Story = {
  args: {
    title: "Optional credentials",
    description: "These are not required but enable additional functionality.",
    children: (
      <div className="rounded-[6px] bg-stone-100 p-4 text-sm text-muted-foreground">
        Optional fields would go here
      </div>
    ),
  },
};
