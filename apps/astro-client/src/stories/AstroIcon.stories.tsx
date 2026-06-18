import type { Meta, StoryObj } from "@storybook/react-vite";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { AstroIcon } from "@/components/ui/astro-icon";

const meta = {
  title: "UI/AstroIcon",
  parameters: { layout: "centered" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <div className="flex flex-col gap-6 p-4">

      {/* Size comparison */}
      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">Sizes</p>
        <div className="flex items-center gap-4">
          <AstroIcon className="h-3 w-3 text-muted-foreground" />
          <AstroIcon className="h-3.5 w-3.5 text-muted-foreground" />
          <AstroIcon className="h-4 w-4 text-muted-foreground" />
          <AstroIcon className="h-5 w-5 text-muted-foreground" />
          <AstroIcon className="h-6 w-6 text-muted-foreground" />
        </div>
      </div>

      {/* Side-by-side with GitHub icon */}
      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">Alongside GitHub icon (h-3.5)</p>
        <div className="flex items-center gap-3">
          <span className="h-3.5 w-3.5">{getIntegrationIcon("github")}</span>
          <AstroIcon className="h-3.5 w-3.5 text-muted-foreground" />
        </div>
      </div>

      {/* In context — the build log subheader */}
      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">In context</p>
        <div className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
          <span className="h-3.5 w-3.5 shrink-0">{getIntegrationIcon("github")}</span>
          <span>a1b2c3d</span>
          <span className="mx-0.5">→</span>
          <AstroIcon className="h-3.5 w-3.5 shrink-0" />
          <span>bld_abc123</span>
        </div>
      </div>

      {/* Color variants */}
      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">Color variants</p>
        <div className="flex items-center gap-4">
          <AstroIcon className="h-5 w-5 text-muted-foreground" />
          <AstroIcon className="h-5 w-5 text-foreground" />
          <AstroIcon className="h-5 w-5 text-violet-500" />
          <AstroIcon className="h-5 w-5 text-teal-500" />
        </div>
      </div>

    </div>
  ),
};
