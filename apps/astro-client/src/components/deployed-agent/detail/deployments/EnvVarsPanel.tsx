import { cn } from "@/lib/utils";
import { Tag, type TagColor } from "@/components/Tag";
import type { MappedEnvVar } from "./history/types";

interface EnvVarsPanelProps {
  vars: MappedEnvVar[];
}

const SOURCE_COLOR: Record<"input" | "injected" | "static", TagColor> = {
  input:    "blue",
  injected: "yellow",
  static:   "default",
};

export function EnvVarsPanel({ vars }: EnvVarsPanelProps) {
  return (
    <div className="bg-stone-50">
      {vars.length === 0 ? (
        <div className="p-4 font-mono text-mono-sm text-faint-foreground">No variables</div>
      ) : (
        vars.map((v, vi) => {
          const sourceKey = v.source === "input" ? "input" : v.source === "injected" ? "injected" : "static";
          return (
            <div
              key={v.key}
              className={cn(
                "flex items-center gap-2.5 px-4 py-[9px]",
                vi < vars.length - 1 && "border-b border-border",
              )}
            >
              <span className="font-mono text-label text-stone-500 shrink-0 select-none">
                {"{}"}
              </span>
              <span
                className={cn(
                  "font-mono text-mono-sm min-w-40 shrink-0",
                  !v.value ? "text-stone-500 line-through" : "text-foreground",
                )}
              >
                {v.key}
              </span>
              <div className="flex-1 flex items-center gap-1.5 min-w-0">
                <span
                  className={cn(
                    "font-mono text-mono-sm truncate",
                    !v.value ? "text-stone-500 italic" : "text-muted-foreground",
                  )}
                >
                  {!v.value ? "empty" : v.secret ? "•••••••••" : v.value}
                </span>
              </div>
              <Tag color={SOURCE_COLOR[sourceKey]} className="shrink-0">{sourceKey}</Tag>
            </div>
          );
        })
      )}
    </div>
  );
}
