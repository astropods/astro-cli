import { SidebarSection } from "./SidebarSection";

export interface SidebarAboutProps {
  description: string;
}

export function SidebarAbout({ description }: SidebarAboutProps) {
  if (!description) return null;

  return (
    <div className="pt-5 mt-5 border-t border-border-strong">
      <SidebarSection title="About">
        <p className="text-[13px] text-foreground leading-[1.55]">{description}</p>
      </SidebarSection>
    </div>
  );
}
