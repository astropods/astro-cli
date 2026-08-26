import { useMemo, useState } from "react";
import { Link } from "react-router";
import { Boxes, CircleAlert, ExternalLink, RefreshCw, Search, UsersRound } from "lucide-react";

import { useAuthorizationResources } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, formatDateTime, truncateUUID } from "@/lib/utils";
import type { AuthorizationResource } from "@/types/admin";

export function ResourcesPage() {
  const resourcesQuery = useAuthorizationResources();
  const [search, setSearch] = useState("");
  const [account, setAccount] = useState("all");
  const [resourceType, setResourceType] = useState("all");
  const [syncState, setSyncState] = useState("all");
  const [errorsOnly, setErrorsOnly] = useState(false);

  const resources = resourcesQuery.data?.resources ?? [];
  const accounts = useMemo(
    () => [...new Set(resources
      .map((resource) => resource.account_name || resource.account_id)
      .filter((value): value is string => Boolean(value)))].sort(),
    [resources],
  );
  const resourceTypes = useMemo(
    () => [...new Set(resources.map((resource) => resource.type).filter(Boolean))].sort(),
    [resources],
  );
  const syncStates = useMemo(
    () => [...new Set(resources.map((resource) => resource.sync_state).filter(Boolean))].sort(),
    [resources],
  );
  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return resources.filter((resource) => {
      const accountLabel = resource.account_name || resource.account_id || "";
      if (account !== "all" && accountLabel !== account) return false;
      if (resourceType !== "all" && resource.type !== resourceType) return false;
      if (syncState !== "all" && resource.sync_state !== syncState) return false;
      if (errorsOnly && !resource.last_error) return false;
      if (!needle) return true;
      return [
        resource.name,
        resource.type,
        resource.external_id,
        resource.workos_resource_id,
        accountLabel,
        resource.last_error,
      ].some((value) => value?.toLowerCase().includes(needle));
    });
  }, [account, errorsOnly, resourceType, resources, search, syncState]);

  if (resourcesQuery.isLoading) return <Skeleton className="h-72 w-full" />;
  if (resourcesQuery.error) {
    return <p className="text-sm text-destructive">Resource inventory failed: {resourcesQuery.error.message}</p>;
  }

  const directAdmins = resources.reduce((total, resource) => total + (resource.direct_admins?.length ?? 0), 0);
  const errorCount = resources.filter((resource) => resource.last_error).length;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-semibold">Resources</h2>
            <span className="rounded-full bg-green-500/10 px-2 py-0.5 text-[10px] font-medium text-green-700 dark:text-green-400">
              Live from WorkOS
            </span>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            WorkOS-backed product resources, synchronization state, and access evidence.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => resourcesQuery.refetch()} disabled={resourcesQuery.isFetching}>
          <RefreshCw className={cn("size-3.5", resourcesQuery.isFetching && "animate-spin")} />
          Refresh
        </Button>
      </div>

      <div className="grid gap-2 sm:grid-cols-3">
        <Stat icon={Boxes} value={resources.length} label="WorkOS resources" />
        <Stat icon={UsersRound} value={directAdmins} label="direct admin assignments" />
        <Stat icon={CircleAlert} value={errorCount} label="sync errors" warn={errorCount > 0} />
      </div>

      <div className="grid gap-2 md:grid-cols-[minmax(240px,1fr)_repeat(3,minmax(140px,auto))_auto]">
        <label className="relative block">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search resource, ID, or account" className="pl-8" />
        </label>
        <Filter value={account} onChange={setAccount} label="All accounts" values={accounts} />
        <Filter value={resourceType} onChange={setResourceType} label="All resource types" values={resourceTypes} />
        <Filter value={syncState} onChange={setSyncState} label="All sync states" values={syncStates} />
        <label className="flex h-9 items-center gap-2 rounded-md border bg-background px-3 text-xs">
          <input type="checkbox" checked={errorsOnly} onChange={(event) => setErrorsOnly(event.target.checked)} />
          Has error
        </label>
      </div>

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full min-w-[1120px] text-[11px]">
          <thead className="glass-subtle">
            <tr className="border-b border-glass-border-honey text-left text-muted-foreground">
              <th className="px-3 py-2 font-medium">Resource</th>
              <th className="px-3 py-2 font-medium">Astro external ID</th>
              <th className="px-3 py-2 font-medium">WorkOS resource ID</th>
              <th className="px-3 py-2 font-medium">Account</th>
              <th className="px-3 py-2 font-medium">Direct admins</th>
              <th className="px-3 py-2 text-right font-medium">Assignments</th>
              <th className="px-3 py-2 font-medium">Created</th>
              <th className="px-3 py-2 font-medium">Sync</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((resource) => <ResourceRow key={resource.workos_resource_id} resource={resource} />)}
          </tbody>
        </table>
        {filtered.length === 0 && (
          <div className="px-4 py-12 text-center text-xs text-muted-foreground">No resources match these filters.</div>
        )}
      </div>
    </div>
  );
}

function ResourceRow({ resource }: { resource: AuthorizationResource }) {
  const resourceHref = `/admin/resources/${encodeURIComponent(resource.type)}/${encodeURIComponent(resource.external_id)}`;
  return (
    <tr className="border-b border-comb-light align-top last:border-0 hover:bg-glass-light">
      <td className="px-3 py-2.5">
        <div className="font-medium">{resource.name || "Unnamed resource"}</div>
        <div className="mt-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">{resource.type}</div>
      </td>
      <td className="px-3 py-2.5 font-mono">
        <Link className="inline-flex items-center gap-1 text-honey-dark hover:underline" to={resourceHref}>
          {resource.external_id}<ExternalLink className="size-2.5" />
        </Link>
      </td>
      <td className="px-3 py-2.5 font-mono" title={resource.workos_resource_id}>{truncateUUID(resource.workos_resource_id)}</td>
      <td className="px-3 py-2.5">
        <div>{resource.account_name || "Unresolved"}</div>
        {resource.account_id && <div className="font-mono text-[10px] text-muted-foreground">{truncateUUID(resource.account_id)}</div>}
      </td>
      <td className="px-3 py-2.5">
        {(resource.direct_admins?.length ?? 0) > 0
          ? resource.direct_admins?.map((admin) => <div key={admin} className="font-mono text-[10px]" title={admin}>{formatDirectAdmin(admin)}</div>)
          : <span className="text-muted-foreground">None</span>}
      </td>
      <td className="px-3 py-2.5 text-right tabular-nums">{resource.assignment_count}</td>
      <td className="px-3 py-2.5 text-muted-foreground">{formatDateTime(resource.created_at)}</td>
      <td className="px-3 py-2.5">
        <span className={cn(
          "rounded-full px-1.5 py-0.5 text-[10px] font-medium",
          resource.last_error ? "bg-destructive/10 text-destructive" : "bg-green-500/10 text-green-700 dark:text-green-400",
        )}>{resource.last_error ? "error" : resource.sync_state}</span>
        {resource.last_error && <div className="mt-1 max-w-52 text-[10px] text-destructive" title={resource.last_error}>{resource.last_error}</div>}
      </td>
    </tr>
  );
}

function formatDirectAdmin(subject: string) {
  const groupPrefix = "group:";
  if (subject.startsWith(groupPrefix)) {
    return `${groupPrefix}${truncateUUID(subject.slice(groupPrefix.length))}`;
  }
  return truncateUUID(subject);
}

function Filter({ value, onChange, label, values }: { value: string; onChange: (value: string) => void; label: string; values: string[] }) {
  return (
    <select className="h-9 rounded-md border bg-background px-2 text-xs" value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="all">{label}</option>
      {values.map((item) => <option key={item} value={item}>{item}</option>)}
    </select>
  );
}

function Stat({ icon: Icon, value, label, warn }: { icon: typeof Boxes; value: number; label: string; warn?: boolean }) {
  return (
    <div className="flex items-center gap-3 rounded-lg glass px-3 py-2.5">
      <span className={cn("flex size-8 items-center justify-center rounded-md bg-honey/10 text-honey-dark", warn && "bg-destructive/10 text-destructive")}>
        <Icon className="size-3.5" />
      </span>
      <div><div className="text-lg font-semibold tabular-nums">{value}</div><div className="text-[10px] text-muted-foreground">{label}</div></div>
    </div>
  );
}
