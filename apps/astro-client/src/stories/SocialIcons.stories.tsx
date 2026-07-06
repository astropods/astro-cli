import type { Meta, StoryObj } from "@storybook/react-vite";
import { XIcon } from "@/components/ui/svgs/xIcon";
import { LinkedInIcon } from "@/components/ui/svgs/linkedinIcon";
import { DiscordIcon } from "@/components/ui/svgs/discordIcon";

const meta = {
  title: "UI/SocialIcons",
  parameters: { layout: "centered" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <div className="flex flex-col gap-6 p-4">

      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">Sizes</p>
        <div className="flex items-center gap-6">
          {([3, 3.5, 4, 5, 6] as const).map((s) => (
            <div key={s} className="flex flex-col items-center gap-2">
              <div className="flex items-center gap-2">
                <XIcon className={`h-${s} w-${s} text-foreground`} />
                <LinkedInIcon className={`h-${s} w-${s} text-foreground`} />
                <DiscordIcon className={`h-${s} w-${s} text-foreground`} />
              </div>
              <span className="text-[10px] text-muted-foreground font-mono">{s}</span>
            </div>
          ))}
        </div>
      </div>

      <div>
        <p className="text-xs text-muted-foreground mb-3 font-mono">In context — share buttons</p>
        <div className="flex items-center gap-1.5">
          <a className="flex items-center gap-1.5 rounded-[4px] border border-border bg-transparent px-2 py-1 font-mono text-[11px] leading-none uppercase tracking-[0.14em] text-muted-foreground">
            Share on <LinkedInIcon className="h-[13px] w-[13px] shrink-0" />
          </a>
          <a className="flex items-center gap-1.5 rounded-[4px] border border-border bg-transparent px-2 py-1 font-mono text-[11px] leading-none uppercase tracking-[0.14em] text-muted-foreground">
            Share on <XIcon className="h-[11px] w-[11px] shrink-0" />
          </a>
          <a className="flex items-center gap-1.5 rounded-[4px] border border-border bg-transparent px-2 py-1 font-mono text-[11px] leading-none uppercase tracking-[0.14em] text-muted-foreground">
            Join <DiscordIcon className="h-[13px] w-[13px] shrink-0" />
          </a>
        </div>
      </div>

    </div>
  ),
};
