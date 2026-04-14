import { Globe } from "lucide-react";
import { cn } from "@/lib/utils";
import { Tag } from "@/components/Tag";
import type { DomainUrl } from "./history/types";

interface DomainsPanelProps {
  urls: DomainUrl[];
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
            <Tag className="shrink-0">{u.type}</Tag>
          )}
        </div>
      ))}
    </div>
  );
}
