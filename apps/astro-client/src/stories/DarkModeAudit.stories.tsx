import React from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { MetricCard } from "@/components/MetricCard";
import { OrgSwitcher } from "@/components/OrgSwitcher";
import { FilterInput } from "@/components/FilterInput";

// Current teal palette (−50% chroma, +20° hue from original)
const tealCurrent = {
  25:  "oklch(98.88% 0.0027 214.137)", 50:  "oklch(97.76% 0.0053 214.137)",
  100: "oklch(92.68% 0.0182 214.362)", 200: "oklch(85.43% 0.0346 214.037)",
  300: "oklch(75.65% 0.0498 213.024)", 400: "oklch(65.79% 0.0509 211.737)",
  500: "oklch(54.95% 0.0452 210.590)", 600: "oklch(45.59% 0.0374 209.997)",
  700: "oklch(38.11% 0.0310 210.373)", 800: "oklch(32.73% 0.0262 212.131)",
  900: "oklch(21.89% 0.0164 215.686)", 950: "oklch(11.05% 0.0066 218.266)",
};

const tealOriginal = {
  25:  "oklch(98.88% 0.0053 194.137)", 50:  "oklch(97.76% 0.0106 194.137)",
  100: "oklch(92.68% 0.0364 194.362)", 200: "oklch(85.43% 0.0692 194.037)",
  300: "oklch(75.65% 0.0997 193.024)", 400: "oklch(65.79% 0.1018 191.737)",
  500: "oklch(54.95% 0.0904 190.590)", 600: "oklch(45.59% 0.0749 189.997)",
  700: "oklch(38.11% 0.0620 190.373)", 800: "oklch(32.73% 0.0525 192.131)",
  900: "oklch(21.89% 0.0328 195.686)", 950: "oklch(11.05% 0.0131 198.266)",
};

const STEPS = [25, 50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <h2 className="text-label font-mono text-faint-foreground uppercase tracking-widest border-b border-border pb-1">{title}</h2>
      <div className="flex flex-wrap gap-3">{children}</div>
    </div>
  );
}

function PaletteStrip({ label, scale }: { label: string; scale: Record<number, string> }) {
  return (
    <div className="space-y-1.5">
      <p className="text-[11px] font-mono text-faint-foreground">{label}</p>
      <div className="flex gap-1">
        {STEPS.map(step => (
          <div key={step} className="flex flex-col items-center gap-1">
            <div className="h-10 w-10 rounded-sm" style={{ background: scale[step] }} />
            <span className="text-[9px] font-mono text-faint-foreground">{step}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function DarkModeAuditStory() {
  return (
    <div className="dark min-h-screen bg-background text-foreground p-8 space-y-10 font-sans">
      <div>
        <h1 className="text-heading-4 font-semibold mb-1">Dark Mode — Changes</h1>
        <p className="text-body-sm text-muted-foreground">Switch toolbar to Dark to preview in context.</p>
      </div>

      {/* ── Teal palette ── */}
      <Section title="Teal palette — updated (−50% chroma, +20° hue)">
        <div className="w-full space-y-4">
          <PaletteStrip label="Before" scale={tealOriginal} />
          <PaletteStrip label="After" scale={tealCurrent} />
          <p className="text-body-sm text-muted-foreground pt-1">
            Light mode uses <strong className="text-foreground">800, 600, 500</strong> · Dark mode uses <strong className="text-foreground">950, 400, 300, 100</strong>
          </p>
        </div>
      </Section>

      {/* ── MetricCard ── */}
      <Section title="MetricCard — bg-white → dark:bg-surface, sparkline → --primary">
        <MetricCard label="Total runs" value="1,204" trend={12.4} higherIsBetter sparkline={[10, 20, 15, 30, 25, 40, 35]} />
        <MetricCard label="Error rate" value="3.2" valueSuffix="%" trend={-8.1} higherIsBetter={false} sparkline={[40, 35, 30, 25, 20, 15, 10]} />
        <MetricCard label="Latency" value="420" valueSuffix="ms" trend={null} />
      </Section>

      {/* ── OrgSwitcher ── */}
      <Section title="OrgSwitcher — dark:hover:bg-stone-700 → inputBase dark:hover:bg-teal-800">
        <div className="w-48">
          <OrgSwitcher activeAccount="testuser" onChange={() => {}} />
        </div>
      </Section>

      {/* ── FilterInput ── */}
      <Section title="FilterInput — icon + placeholder dark:text-teal-25">
        <FilterInput placeholder="Find a blueprint..." className="w-72" />
      </Section>
    </div>
  );
}

const meta = {
  title: "Audit/Dark Mode",
  component: DarkModeAuditStory,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DarkModeAuditStory>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Audit: Story = {};
