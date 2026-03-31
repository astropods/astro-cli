import type { Meta, StoryObj } from "@storybook/react-vite";
import { Spinner } from "@/components/ui/spinner";

const meta = {
  title: "Design System/Primitives/Spinner",
  component: Spinner,
  argTypes: {
    size: { control: { type: "range", min: 12, max: 64, step: 4 } },
    delay: { control: { type: "number" }, description: "ms before spinner becomes visible (CSS animation, no JS timer)" },
  },
} satisfies Meta<typeof Spinner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { size: 20 },
};

export const Small: Story = {
  args: { size: 14 },
};

export const Large: Story = {
  args: { size: 32 },
};

export const AllSizes: Story = {
  args: {},
  render: () => (
    <div style={{ display: "flex", alignItems: "center", gap: 24 }}>
      <Spinner size={14} />
      <Spinner size={20} />
      <Spinner size={32} />
      <Spinner size={48} />
    </div>
  ),
};

export const Centered: Story = {
  args: { size: 20 },
  render: () => (
    <div style={{ display: "flex", height: 200, alignItems: "center", justifyContent: "center", border: "1px dashed var(--border)", borderRadius: 8 }}>
      <Spinner size={20} />
    </div>
  ),
};

export const WithDelay: Story = {
  args: { size: 20, delay: 2000 },
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <p style={{ fontSize: 13, color: "var(--muted-foreground)", margin: 0 }}>
        Spinner renders immediately but stays invisible for 2s — reload to see the effect.
      </p>
      <div style={{ display: "flex", height: 120, alignItems: "center", justifyContent: "center", border: "1px dashed var(--border)", borderRadius: 8 }}>
        <Spinner size={20} delay={2000} />
      </div>
    </div>
  ),
};
