import { cn } from "@/lib/utils";

interface EnvVar {
  key: string;
  value: string;
  secret: boolean;
  source: string;
}

interface EnvVarsPanelProps {
  vars: EnvVar[];
}

function isSensitiveEnvVar(key: string, value: string, source: string): boolean {
  if (source.startsWith("secret:")) return true;

  const upperKey = key.toUpperCase();
  const keyLooksSensitive =
    upperKey.includes("KEY") ||
    upperKey.includes("TOKEN") ||
    upperKey.includes("SECRET") ||
    upperKey.includes("PASSWORD") ||
    upperKey.includes("PASSWD") ||
    upperKey.includes("PRIVATE") ||
    upperKey.includes("CREDENTIAL") ||
    upperKey.includes("AUTH") ||
    upperKey.includes("DSN") ||
    upperKey.includes("WEBHOOK");

  const valueLooksSensitive =
    value.startsWith("sk-") ||
    value.startsWith("secret:") ||
    value.includes("••");

  return keyLooksSensitive || valueLooksSensitive;
}

const SOURCE_STYLES = {
  input: "bg-teal-600/10 text-teal-600",
  injected: "bg-amber-700/10 text-amber-800",
  static: "bg-muted text-stone-500",
} as const;

export function EnvVarsPanel({ vars }: EnvVarsPanelProps) {
  return (
    <div className="bg-stone-50">
      {vars.length === 0 ? (
        <div className="p-4 font-mono text-mono-sm text-faint-foreground">No variables</div>
      ) : (
        vars.map((v, vi) => {
          const isSecret =
            v.secret || v.value.startsWith("sk-") || v.value.startsWith("secret:") || v.value.includes("••");
          const sourceKey = v.source === "input" ? "input" : v.source === "injected" ? "injected" : "static";
          const sourceLabel = sourceKey;
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
                  {!v.value ? "empty" : (isSecret || isSensitiveEnvVar(v.key, v.value, v.source)) ? "•••••••••" : v.value}
                </span>
              </div>
              <span className={cn("font-mono text-label tracking-[0.08em] px-1.5 py-0.5 rounded shrink-0", SOURCE_STYLES[sourceKey])}>
                {sourceLabel}
              </span>
            </div>
          );
        })
      )}
    </div>
  );
}
