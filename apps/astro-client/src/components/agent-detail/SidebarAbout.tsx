import { SidebarSection } from "./SidebarSection";

export interface SidebarAboutProps {
  description: string;
}

export function SidebarAbout({ description }: SidebarAboutProps) {
  if (!description) return null;

  return (
    <SidebarSection title="About">
      <p className="text-[13px] text-foreground leading-[1.55]">{description}</p>
    </SidebarSection>
  );
}
