import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Grid2x2, List, LayoutList, Columns3 } from "lucide-react";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const meta = {
  title: "Design System/Primitives/ToggleGroup",
  component: ToggleGroup,
  decorators: [
    (Story) => (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ToggleGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    type: "single",
    defaultValue: "grid",
  },
  render: (args) => (
    <ToggleGroup {...args}>
      <ToggleGroupItem value="list" aria-label="List view" tooltip="List view">
        <List className="h-4 w-4" />
      </ToggleGroupItem>
      <ToggleGroupItem value="grid" aria-label="Grid view" tooltip="Grid view">
        <Grid2x2 className="h-4 w-4" />
      </ToggleGroupItem>
    </ToggleGroup>
  ),
};

export const ThreeOptions: Story = {
  name: "Three Options",
  args: {
    type: "single",
    defaultValue: "grid",
  },
  render: (args) => (
    <ToggleGroup {...args}>
      <ToggleGroupItem value="list" aria-label="List view" tooltip="List view">
        <List className="h-4 w-4" />
      </ToggleGroupItem>
      <ToggleGroupItem value="detail" aria-label="Detail view" tooltip="Detail view">
        <LayoutList className="h-4 w-4" />
      </ToggleGroupItem>
      <ToggleGroupItem value="grid" aria-label="Grid view">
        <Columns3 className="h-4 w-4" />
      </ToggleGroupItem>
    </ToggleGroup>
  ),
};

export const Interactive: Story = {
  args: { type: "single" },
  render: () => {
    const [value, setValue] = useState("grid");

    return (
      <div className="flex items-center gap-4">
        <ToggleGroup
          type="single"
          value={value}
          onValueChange={(v) => {
            if (v) setValue(v);
          }}
        >
          <ToggleGroupItem value="list" aria-label="List view">
            <List className="h-4 w-4" />
          </ToggleGroupItem>
          <ToggleGroupItem value="grid" aria-label="Grid view">
            <Grid2x2 className="h-4 w-4" />
          </ToggleGroupItem>
        </ToggleGroup>
        <span className="text-sm text-muted-foreground">Selected: {value}</span>
      </div>
    );
  },
};
