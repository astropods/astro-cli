import type { Meta, StoryObj } from "@storybook/react-vite";
import { isValidElement } from "react";
import { integrationIconMap } from "@/lib/integrationIcons";
import { IntegrationBadge } from "@/components/IntegrationBadge";

function IntegrationIconCatalog() {
  const seen = new Set<unknown>();
  const unique = Object.entries(integrationIconMap).filter(([, icon]) => {
    const type = isValidElement(icon) ? icon.type : icon;
    if (seen.has(type)) return false;
    seen.add(type);
    return true;
  });

  return (
    <div className="flex flex-wrap gap-4">
      {unique.map(([name, icon]) => (
        <div key={name} className="flex flex-col items-center gap-2">
          <IntegrationBadge name={name} icon={icon} />
          <span className="text-xs text-muted-foreground">{name}</span>
        </div>
      ))}
    </div>
  );
}

const meta = {
  title: "Components/Integration/IntegrationIcons",
  component: IntegrationIconCatalog,
} satisfies Meta<typeof IntegrationIconCatalog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Catalog: Story = {};
