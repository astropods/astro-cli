import { useMemo, useState } from "react";
import { X } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  MultiSelect,
  MultiSelectContent,
  MultiSelectItem,
  MultiSelectList,
  MultiSelectTrigger,
} from "@/components/ui/multi-select";
import { cn } from "@/lib/utils";
import type { AppScope } from "@/lib/api";

export interface AppScopePickerProps {
  scopes: AppScope[];
  value: string[];
  onChange: (next: string[]) => void;
  loading?: boolean;
  error?: string;
}

// A permission's resource comes from WorkOS when it is scoped to one. Otherwise
// the slug's own prefix is the resource, which is the convention WorkOS
// recommends for naming them (resource:action).
function groupByResource(scopes: AppScope[]): [string, AppScope[]][] {
  const groups = new Map<string, AppScope[]>();
  for (const scope of scopes) {
    const key = scope.resource_type || scope.slug.split(":")[0] || "other";
    groups.set(key, [...(groups.get(key) ?? []), scope]);
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
}

export function AppScopePicker({ scopes, value, onChange, loading, error }: AppScopePickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const bySlug = useMemo(() => new Map(scopes.map((s) => [s.slug, s])), [scopes]);
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return scopes;
    return scopes.filter(
      (s) =>
        s.slug.toLowerCase().includes(q) ||
        s.name.toLowerCase().includes(q) ||
        s.description?.toLowerCase().includes(q),
    );
  }, [scopes, query]);
  const groups = useMemo(() => groupByResource(matches), [matches]);

  if (error) {
    return <p className="text-body-sm text-muted-foreground">{error}</p>;
  }
  if (loading) {
    return <p className="text-body-sm text-muted-foreground">Loading scopes…</p>;
  }
  if (scopes.length === 0) {
    return (
      <p className="text-body-sm text-muted-foreground">
        No scopes are configured for this environment yet, so an app is created without any and is
        refused by every scoped endpoint.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <MultiSelect value={value} onValueChange={onChange} open={open} onOpenChange={setOpen}>
        <MultiSelectTrigger aria-label="Select scopes">
          <span className={cn("truncate", value.length === 0 && "text-faint-foreground")}>
            {value.length === 0
              ? "Select scopes"
              : `${value.length} scope${value.length === 1 ? "" : "s"} selected`}
          </span>
        </MultiSelectTrigger>
        <MultiSelectContent className="w-[22rem]">
          <div className="border-b border-border p-2">
            <Input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search scopes…"
              className="h-8"
            />
          </div>
          <MultiSelectList>
            {matches.length === 0 ? (
              <p className="px-3 py-2 text-body-sm text-muted-foreground">No scopes match that.</p>
            ) : (
              groups.map(([resource, entries]) => (
                <div key={resource}>
                  <p className="px-3 pt-2 pb-1 text-[11px] uppercase tracking-wide text-faint-foreground">
                    {resource}
                  </p>
                  {entries.map((scope) => (
                    <MultiSelectItem key={scope.slug} value={scope.slug}>
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate font-mono text-xs text-foreground">
                          {scope.slug}
                        </span>
                        <span className="truncate text-[11px] text-muted-foreground">
                          {scope.description || scope.name}
                        </span>
                      </span>
                    </MultiSelectItem>
                  ))}
                </div>
              ))
            )}
          </MultiSelectList>
        </MultiSelectContent>
      </MultiSelect>

      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {value.map((slug) => (
            <li key={slug}>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card py-1 pr-1.5 pl-2.5">
                <span
                  className="font-mono text-xs text-foreground"
                  title={bySlug.get(slug)?.description || bySlug.get(slug)?.name}
                >
                  {slug}
                </span>
                <button
                  type="button"
                  aria-label={`Remove ${slug}`}
                  onClick={() => onChange(value.filter((s) => s !== slug))}
                  className="cursor-pointer rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <X className="size-3" />
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
