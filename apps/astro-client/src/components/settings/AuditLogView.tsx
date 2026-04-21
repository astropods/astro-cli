import { useState, useMemo } from "react";
import { Loader2, ScrollText, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { FilterInput } from "@/components/FilterInput";
import { UserAvatar } from "@/components/UserAvatar";
import { useAuditLog, useAuditLogFilters, useAccountMembers } from "@/api/queries";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { formatRelativeTime } from "@/lib/deployment-utils";
import type { AuditLogEntry, AccountMember } from "@/lib/api";

const NO_FILTER = "__all__";

// ── Actor resolution ─────────────────────────────────────────────────

type MemberMap = Map<string, AccountMember>;

function resolveActor(actor: { id: string; type: string }, members: MemberMap) {
  if (actor.type === "system") return { name: "System", handle: "system" };
  if (actor.type === "admin") return { name: actor.id, handle: actor.id };
  const member = members.get(actor.id);
  if (member) {
    const name = member.display_name || member.username || actor.id;
    const handle = member.username || actor.id;
    return { name, handle };
  }
  return { name: actor.id, handle: actor.id };
}

// ── Table row ────────────────────────────────────────────────────────

const GRID_COLS = "grid-cols-[1.5fr_0.5fr_0.6fr_0.7fr]";

function EntryRow({
  entry,
  isLast,
  members,
}: {
  entry: AuditLogEntry;
  isLast: boolean;
  members: MemberMap;
}) {
  const actor = resolveActor(entry.actor, members);

  return (
    <div
      className={`grid ${GRID_COLS} gap-x-3 px-4 items-center hover:bg-muted/40 transition-colors ${isLast ? "" : "border-b border-border"}`}
    >
      <div className="py-3 min-w-0">
        {entry.description && (
          <span className="text-body-sm text-foreground truncate block">
            {entry.description}
          </span>
        )}
        <span className="text-mono-sm text-muted-foreground font-mono truncate block">
          {entry.action}
        </span>
      </div>

      <div className="py-3 min-w-0">
        <span className="text-body-sm text-foreground truncate block">
          {entry.resource.name || entry.resource.id}
        </span>
        <span className="text-mono-sm text-muted-foreground font-mono truncate block">
          {entry.resource.type}
        </span>
      </div>

      <div className="py-3 min-w-0">
        <div className="flex items-center gap-2 min-w-0">
          {entry.actor.type === "user" && (
            <UserAvatar
              handle={actor.handle}
              name={actor.name}
              className="size-5 shrink-0"
            />
          )}
          <span className="text-body-sm text-foreground truncate">
            {actor.name}
          </span>
        </div>
      </div>

      <div className="py-3">
        <span className="text-body-sm text-muted-foreground">
          {formatRelativeTime(entry.created_at)}
        </span>
      </div>
    </div>
  );
}

// ── Main view ────────────────────────────────────────────────────────

export function AuditLogView({
  account,
  subtitle,
}: {
  account: string;
  subtitle: string;
}) {
  const [resourceType, setResourceType] = useState("");
  const [action, setAction] = useState("");
  const [actorId, setActorId] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 250);

  const {
    data,
    isLoading,
    error,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
  } = useAuditLog(account, {
    limit: 50,
    resource_type: resourceType || undefined,
    action: action || undefined,
    actor_id: actorId || undefined,
  });

  const { data: filtersData } = useAuditLogFilters(account);
  const { data: membersData } = useAccountMembers(account);

  const memberMap = useMemo<MemberMap>(() => {
    const map = new Map<string, AccountMember>();
    for (const m of membersData?.members ?? []) {
      map.set(m.user_id, m);
    }
    return map;
  }, [membersData]);

  const allEntries = useMemo(
    () => data?.pages.flatMap((p) => p.entries) ?? [],
    [data],
  );

  const entries = useMemo(() => {
    if (!debouncedSearch) return allEntries;
    const q = debouncedSearch.toLowerCase();
    return allEntries.filter(
      (e) =>
        e.description?.toLowerCase().includes(q) ||
        e.action.toLowerCase().includes(q) ||
        e.resource.name?.toLowerCase().includes(q) ||
        e.resource.id.toLowerCase().includes(q),
    );
  }, [allEntries, debouncedSearch]);

  const resourceTypes = filtersData?.resource_types ?? [];
  const allActions = filtersData?.actions ?? [];

  const actorOptions = useMemo(() => {
    const members = membersData?.members ?? [];
    return members.map((m) => ({
      value: m.user_id,
      label: m.display_name || m.username || m.user_id,
    }));
  }, [membersData]);

  const hasActiveFilters = !!(resourceType || action || actorId || search);

  const clearAll = () => {
    setResourceType("");
    setAction("");
    setActorId("");
    setSearch("");
  };

  const fromSelect = (val: string) => (val === NO_FILTER ? "" : val);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-heading-2 text-foreground">Audit Log</h2>
        <p className="text-[13px] text-muted-foreground mt-1">{subtitle}</p>
      </div>

      <Separator />

      {/* Filter toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <FilterInput
          placeholder="Search events..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          containerClassName="w-56"
        />

        <Select
          value={resourceType || NO_FILTER}
          onValueChange={(val) => setResourceType(fromSelect(val))}
        >
          <SelectTrigger className="h-9 w-[150px] text-body-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NO_FILTER}>All resources</SelectItem>
            {resourceTypes.map((rt) => (
              <SelectItem key={rt} value={rt}>
                {rt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={action || NO_FILTER}
          onValueChange={(val) => setAction(fromSelect(val))}
        >
          <SelectTrigger className="h-9 w-[210px] text-body-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NO_FILTER}>All actions</SelectItem>
            {allActions.map((a) => (
              <SelectItem key={a} value={a}>
                {a}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {actorOptions.length > 0 && (
          <Select
            value={actorId || NO_FILTER}
            onValueChange={(val) => setActorId(fromSelect(val))}
          >
            <SelectTrigger className="h-9 w-[160px] text-body-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_FILTER}>All actors</SelectItem>
              {actorOptions.map((a) => (
                <SelectItem key={a.value} value={a.value}>
                  {a.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {hasActiveFilters && (
          <Button variant="ghost" size="sm" onClick={clearAll}>
            <X className="size-3" />
            Clear
          </Button>
        )}
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <p className="text-[13px] text-muted-foreground py-4">
          Failed to load audit log.
        </p>
      ) : entries.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
          <div className="flex justify-center mb-3 text-muted-foreground">
            <ScrollText className="size-6" />
          </div>
          <p className="text-sm font-medium text-foreground">
            {hasActiveFilters ? "No matching events" : "No events yet"}
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            {hasActiveFilters
              ? "Try adjusting your filters"
              : "Actions taken in this account will appear here"}
          </p>
        </div>
      ) : (
        <>
          {/* Results summary */}
          <div className="text-[13px] text-muted-foreground">
            Showing {entries.length} {entries.length === 1 ? "entry" : "entries"}
            {hasNextPage && !debouncedSearch ? "+" : ""}
          </div>

          <div className="rounded-[10px] border border-border overflow-hidden">
            <div
              className={`grid ${GRID_COLS} gap-x-3 px-4 border-b border-border bg-muted`}
            >
              {["Event", "Resource", "Actor", "Time"].map((h) => (
                <div
                  key={h}
                  className="font-mono text-label tracking-wider text-faint-foreground py-2.5 uppercase text-left"
                >
                  {h}
                </div>
              ))}
            </div>
            <div className="bg-surface">
              {entries.map((entry, i) => (
                <EntryRow
                  key={entry.id}
                  entry={entry}
                  isLast={i === entries.length - 1}
                  members={memberMap}
                />
              ))}
            </div>
          </div>

          {/* Pagination — only show when not searching (search is client-side) */}
          {hasNextPage && !debouncedSearch && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                size="sm"
                disabled={isFetchingNextPage}
                onClick={() => fetchNextPage()}
              >
                {isFetchingNextPage ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  "Load more"
                )}
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
