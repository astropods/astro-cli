import { cn } from "@/lib/utils";
import { Tag, type TagColor } from "@/components/Tag";
import type { MappedEnvVar } from "./history/types";

interface EnvVarsPanelProps {
  vars: MappedEnvVar[];
}

// Authoritative `source` values come from deployment_build_env (server-
// emitted). Legacy strings ("input", "injected", "static") and From-based
// fallbacks ("secret:...", "configmap:...") are still handled for older
// deployments without rows yet.
const SOURCE_COLOR: Record<string, TagColor> = {
  // Authoritative
  user_var:        "blue",    // user-declared in the deploy form
  platform_meta:   "default", // ASTRO_AGENT_*
  service_url:     "default", // knowledge/integration HOST/PORT/URL
  knowledge_cred:  "yellow",  // auto-managed provider creds
  auth_token:      "yellow",  // ASTRO_AUTHZ_TOKEN
  adapter_config:  "default", // SLACK_CONFIG and friends
  derived:         "default",
  // Legacy
  input:           "blue",
  injected:        "yellow",
  static:          "default",
};

function sourceColor(source: string): TagColor {
  if (SOURCE_COLOR[source] !== undefined) return SOURCE_COLOR[source];
  if (source.startsWith("secret:")) return "yellow";
  if (source.startsWith("configmap:")) return "default";
  return "default";
}

function sourceLabel(source: string): string {
  // Authoritative sources are snake_case; render with spaces for the badge.
  const map: Record<string, string> = {
    user_var:       "input",
    platform_meta:  "platform",
    service_url:    "service",
    knowledge_cred: "credential",
    auth_token:     "credential",
    adapter_config: "adapter",
    derived:        "derived",
  };
  if (map[source]) return map[source];
  // Legacy values render as themselves.
  if (source === "input" || source === "injected" || source === "static") return source;
  // From-based fallbacks: just say "static" for the badge.
  return "static";
}

export function EnvVarsPanel({ vars }: EnvVarsPanelProps) {
  return (
    <div className="bg-background">
      {vars.length === 0 ? (
        <div className="p-4 font-mono text-mono-sm text-faint-foreground">No variables</div>
      ) : (
        vars.map((v, vi) => {
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
              <Tag color={sourceColor(v.source)} className="shrink-0">{sourceLabel(v.source)}</Tag>
            </div>
          );
        })
      )}
    </div>
  );
}
