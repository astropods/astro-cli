import type { Meta, StoryObj } from "@storybook/react-vite"

function TypeScale() {
  return null
}

const meta = {
  title: "Design System/Theme/Typography",
  component: TypeScale,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof TypeScale>

export default meta
type Story = StoryObj<typeof meta>

export const FullScale: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      {[
        { name: "Display", className: "text-display", sample: "Hero Headline" },
        { name: "Heading 1", className: "text-heading-1", sample: "Page Title" },
        { name: "Heading 2", className: "text-heading-2", sample: "Section Heading" },
        { name: "Heading 3", className: "text-heading-3", sample: "Subsection Heading" },
        { name: "Heading 4", className: "text-heading-4", sample: "Group Heading" },
        { name: "Body", className: "text-body", sample: "Body copy used for descriptions and general content. The quick brown fox jumps over the lazy dog." },
        { name: "Body Small", className: "text-body-sm", sample: "Small body text for supporting information and secondary content." },
        { name: "Label", className: "text-label font-mono uppercase", sample: "Status Label" },
        { name: "Mono MD", className: "text-mono-md font-mono", sample: "1,024 requests" },
        { name: "Mono SM", className: "text-mono-sm font-mono", sample: "metadata-key" },
      ].map((v) => (
        <div key={v.name} className="flex flex-col gap-1 border-b border-border pb-6 last:border-0">
          <div className="flex items-baseline gap-3">
            <span className="text-body-sm font-semibold text-foreground">{v.name}</span>
            <code className="text-mono-sm font-mono text-muted-foreground">{v.className}</code>
          </div>
          <p className={`${v.className} text-foreground`}>{v.sample}</p>
        </div>
      ))}
    </div>
  ),
}

export const Display: Story = {
  render: () => <p className="text-display text-foreground">Hero Headline</p>,
}

export const Heading1: Story = {
  render: () => <p className="text-heading-1 text-foreground">Page Title</p>,
}

export const Heading2: Story = {
  render: () => <p className="text-heading-2 text-foreground">Section Heading</p>,
}

export const Heading3: Story = {
  render: () => <p className="text-heading-3 text-foreground">Subsection Heading</p>,
}

export const Heading4: Story = {
  render: () => <p className="text-heading-4 text-foreground">Group Heading</p>,
}

export const Body: Story = {
  render: () => (
    <p className="text-body text-foreground max-w-prose">
      Body copy used for descriptions and general content. The quick brown fox jumps over the lazy dog.
    </p>
  ),
}

export const BodySmall: Story = {
  render: () => (
    <p className="text-body-sm text-foreground max-w-prose">
      Small body text for supporting information and secondary content.
    </p>
  ),
}

export const Label: Story = {
  render: () => <p className="text-label font-mono uppercase text-foreground">Status Label</p>,
}

export const MonoMd: Story = {
  render: () => <p className="text-mono-md font-mono text-foreground">1,024 requests</p>,
}

export const MonoSm: Story = {
  render: () => <p className="text-mono-sm font-mono text-foreground">metadata-key</p>,
}
