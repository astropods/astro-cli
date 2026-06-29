import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResizeCornerHandle } from "@/components/ui/resize-corner-handle";

const meta = {
  title: "Design System/Primitives/ResizeCornerHandle",
  component: ResizeCornerHandle,
  parameters: { layout: "centered" },
  argTypes: {
    className: { control: "text" },
  },
} satisfies Meta<typeof ResizeCornerHandle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {},
};

export const Large: Story = {
  args: {
    className: "size-6 text-foreground",
  },
};

export const InResizeButton: Story = {
  render: () => (
    <div className="relative h-28 w-56 rounded-md border border-border bg-card p-4 text-body-sm text-muted-foreground">
      Resizable content
      <button
        type="button"
        aria-label="Resize content"
        className="absolute bottom-0 right-0 size-3 cursor-ns-resize"
      >
        <ResizeCornerHandle className="absolute bottom-px right-px" />
      </button>
    </div>
  ),
};
