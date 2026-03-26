import { Globe } from "lucide-react";
import { cn } from "@/lib/utils";

interface DomainsPanelProps {
  urls: { name: string; url: string; type?: string }[];
}

export function DomainsPanel({ urls }: DomainsPanelProps) {
  return (
    <div className="bg-stone-50">
      {urls.map((u, i) => (
        <div
          key={u.url}
          className={cn(
            "flex items-center gap-2.5 px-4 py-[9px]",
            i < urls.length - 1 && "border-b border-border",
          )}
        >
          <Globe size={14} className="shrink-0 text-faint-foreground" />
          <span className="font-sans text-body text-foreground truncate flex-1">
            {u.url}
          </span>
          {u.type && (
            <span className="font-mono text-label tracking-[0.08em] px-1.5 py-0.5 rounded bg-muted text-stone-500 shrink-0">
              {u.type}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
