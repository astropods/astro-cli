import { SidebarSection } from "./SidebarSection";

export interface SidebarAuthorProps {
  name: string;
  handle: string;
  profilePictureUrl?: string;
}

export function SidebarAuthor({
  name,
  handle,
  profilePictureUrl,
}: SidebarAuthorProps) {
  const initial = name.charAt(0).toUpperCase();

  return (
    <div className="pt-5 mt-5 border-t border-border-strong">
      <SidebarSection title="Created by">
        <div className="flex items-center gap-3">
          {profilePictureUrl ? (
            <img
              src={profilePictureUrl}
              alt={name}
              className="h-9 w-9 shrink-0 rounded-full object-cover"
            />
          ) : (
            <div className="flex h-9 w-9 items-center justify-center rounded-full bg-stone-300 text-sm font-semibold text-muted-foreground dark:bg-teal-900">
              {initial}
            </div>
          )}
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-foreground">{name}</span>
            <span className="text-[11px] text-[var(--faint-foreground)] font-mono">
              @{handle}
            </span>
          </div>
        </div>
      </SidebarSection>
    </div>
  );
}
