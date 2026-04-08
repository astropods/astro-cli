import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SidePanel } from "@/components/deployed-agent/detail/SidePanel";

const meta: Meta<typeof SidePanel> = {
  title: "DeployedAgent/SidePanel",
  component: SidePanel,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof SidePanel>;

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen bg-muted font-sans text-foreground">
      <div className="flex flex-1 items-center justify-center text-sm text-faint-foreground">
        Main content area
      </div>
      {children}
    </div>
  );
}

function PanelContent({ label }: { label: string }) {
  return (
    <div className="flex h-full w-full flex-col border-l border-border bg-surface">
      <div className="flex h-[63px] shrink-0 items-center border-b border-border px-5 font-medium text-sm">
        {label}
      </div>
      <div className="flex-1 p-5 text-sm text-muted-foreground">
        Panel content goes here. Drag the left edge to resize.
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => (
    <Shell>
      <SidePanel open>
        <PanelContent label="Resizable panel (default)" />
      </SidePanel>
    </Shell>
  ),
};

export const NotResizable: Story = {
  render: () => (
    <Shell>
      <SidePanel open resizable={false}>
        <PanelContent label="Fixed-width panel (resizable=false)" />
      </SidePanel>
    </Shell>
  ),
};

export const Closed: Story = {
  render: () => (
    <Shell>
      <SidePanel open={false}>
        <PanelContent label="This should not be visible" />
      </SidePanel>
    </Shell>
  ),
};

export const ToggleOpen: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <Shell>
        <div className="absolute top-4 left-4 z-50">
          <button
            onClick={() => setOpen((o) => !o)}
            className="rounded border border-border bg-surface px-3 py-1.5 text-sm font-medium hover:bg-muted"
          >
            {open ? "Close panel" : "Open panel"}
          </button>
        </div>
        <SidePanel open={open}>
          <PanelContent label="Toggle me" />
        </SidePanel>
      </Shell>
    );
  },
};

export const WidthObserver: Story = {
  render: () => {
    const [width, setWidth] = useState(420);
    return (
      <Shell>
        <div className="absolute top-4 left-4 z-50 rounded border border-border bg-surface px-3 py-1.5 text-xs font-mono text-muted-foreground">
          panelWidth: {width}px
        </div>
        <SidePanel open onWidthChange={setWidth}>
          <PanelContent label="Width observer" />
        </SidePanel>
      </Shell>
    );
  },
};
