import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import {
  MyAgentsHeader,
  type ViewMode,
} from "@/components/MyAgentsHeader";

const meta = {
  title: "Layout/MyAgentsHeader",
  component: MyAgentsHeader,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-[900px] p-6">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof MyAgentsHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    filter: "",
    onFilterChange: () => {},
    viewMode: "grid",
    onViewModeChange: () => {},
  },
};

export const ListView: Story = {
  name: "List View Selected",
  args: {
    filter: "",
    onFilterChange: () => {},
    viewMode: "list",
    onViewModeChange: () => {},
  },
};

export const WithFilter: Story = {
  name: "With Filter Query",
  args: {
    filter: "incident",
    onFilterChange: () => {},
    viewMode: "grid",
    onViewModeChange: () => {},
  },
};

export const Interactive: Story = {
  args: {
    filter: "",
    onFilterChange: () => {},
    viewMode: "grid",
    onViewModeChange: () => {},
  },
  render: () => {
    const [filter, setFilter] = useState("");
    const [viewMode, setViewMode] = useState<ViewMode>("grid");

    return (
      <MyAgentsHeader
        filter={filter}
        onFilterChange={setFilter}
        viewMode={viewMode}
        onViewModeChange={setViewMode}

      />
    );
  },
};
