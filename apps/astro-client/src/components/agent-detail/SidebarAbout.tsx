import { SidebarSection } from "./SidebarSection";

export interface SidebarAboutProps {
  description: string;
}

export function SidebarAbout({ description }: SidebarAboutProps) {
  if (!description) return null;

  return (
    <div className="pt-5 mt-5 border-t border-border">
      <SidebarSection title="About">
        <p className="text-sm text-foreground leading-snug">{description}</p>
      </SidebarSection>
    </div>
  );
}
