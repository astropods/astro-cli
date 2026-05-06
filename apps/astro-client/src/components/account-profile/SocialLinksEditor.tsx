import { Link2 } from "lucide-react";
import { Input, inputBase, inputFocusWithin } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { detectSocialLink } from "@/lib/social-links";

interface SocialLinksEditorProps {
  links: [string, string, string, string];
  onChange: (links: [string, string, string, string]) => void;
  /** Compact mode uses smaller inputs — for the inline profile sidebar editor. */
  compact?: boolean;
}

export function SocialLinksEditor({ links, onChange, compact = false }: SocialLinksEditorProps) {
  return (
    <div className="flex flex-col gap-3">
      {links.map((value, i) => {
        const detected = detectSocialLink(value);
        const icon = detected?.icon ?? <Link2 className="size-3.5 opacity-40" />;
        return (
          <div
            key={i}
            className={cn(
              "flex items-center",
              inputBase,
              inputFocusWithin,
              "px-0",
              compact && "h-8",
            )}
          >
            <span className="flex shrink-0 items-center justify-center pl-3 pr-2 text-muted-foreground">
              {icon}
            </span>
            <span className="shrink-0 select-none text-border">|</span>
            <Input
              value={value}
              onChange={(e) => {
                const next = [...links] as [string, string, string, string];
                next[i] = e.target.value;
                onChange(next);
              }}
              placeholder={`Link to social profile ${i + 1}`}
              className={cn(
                "min-w-0 flex-1 h-auto rounded-none border-0 bg-transparent shadow-none focus-visible:ring-0 py-2 pl-2.5 pr-3.5 text-left",
                compact ? "text-body-sm" : "text-body",
              )}
            />
          </div>
        );
      })}
    </div>
  );
}
