import { useMemo, useState } from "react";
import { Link } from "react-router";
import {
  Boxes,
  ChevronDown,
  CircleAlert,
  ExternalLink,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
  UserRound,
  UsersRound,
} from "lucide-react";

import {
  useAuthorizationOperations,
  useAuthorizationResources,
  useStartAuthorizationResourceReset,
} from "@/api/admin";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, formatDateTime, mutationErrorMessage, truncateUUID } from "@/lib/utils";
import type { AuthorizationOperation, AuthorizationResource } from "@/types/admin";

type AccountOption = { id: string; name: string };

export function ResourcesPage() {
  const resourcesQuery = useAuthorizationResources();
  const operationsQuery = useAuthorizationOperations();
  const startReset = useStartAuthorizationResourceReset();
  const [search, setSearch] = useState("");
  const [account, setAccount] = useState("all");
  const [resourceType, setResourceType] = useState("all");
  const [syncState, setSyncState] = useState("all");
  const [errorsOnly, setErrorsOnly] = useState(false);

  const resources = resourcesQuery.data?.resources ?? [];
  const operations = operationsQuery.data?.operations ?? [];
  const accounts = useMemo(
    () => [...resources.reduce((result, resource) => {
      if (resource.account_id) {
        result.set(resource.account_id, resource.account_name || resource.account_id);
      }
      return result;
    }, new Map<string, string>())]
      .map(([id, name]) => ({ id, name }))
      .sort((left, right) => left.name.localeCompare(right.name)),
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
      if (account !== "all" && resource.account_id !== account) return false;
      if (resourceType !== "all" && resource.type !== resourceType) return false;
      if (syncState !== "all" && resource.sync_state !== syncState) return false;
      if (errorsOnly && !resource.last_error) return false;
      if (!needle) return true;
      const resourceMatch = [
        resource.name,
        resource.type,
        resource.external_id,
        resource.workos_resource_id,
        accountLabel,
        resource.last_error,
      ].some((value) => value?.toLowerCase().includes(needle));
      const assignmentMatch = resource.assignments?.some((assignment) =>
        [assignment.subject_label, assignment.subject_id, assignment.role].some((value) => value.toLowerCase().includes(needle)),
      );
      return resourceMatch || assignmentMatch;
    });
  }, [account, errorsOnly, resourceType, resources, search, syncState]);

  if (resourcesQuery.isLoading) return <Skeleton className="h-72 w-full" />;
  if (resourcesQuery.error) {
    return <p className="text-sm text-destructive">Resource inventory failed: {resourcesQuery.error.message}</p>;
  }

  const activeOperation = operations.find((operation) => operation.status === "queued" || operation.status === "running");
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
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => resourcesQuery.refetch()} disabled={resourcesQuery.isFetching}>
            <RefreshCw className={cn("size-3.5", resourcesQuery.isFetching && "animate-spin")} />
            Refresh
          </Button>
          <ResetDialog
            enabled={resourcesQuery.data?.reset_enabled ?? false}
            accounts={accounts}
            operations={operations}
            activeOperation={activeOperation}
            startReset={startReset}
          />
        </div>
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
        <Filter value={account} onChange={setAccount} label="All accounts" options={accounts.map((current) => ({ value: current.id, label: current.name }))} />
        <Filter value={resourceType} onChange={setResourceType} label="All resource types" options={resourceTypes.map((current) => ({ value: current, label: current }))} />
        <Filter value={syncState} onChange={setSyncState} label="All sync states" options={syncStates.map((current) => ({ value: current, label: current }))} />
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

      <Operations operations={operations} accounts={accounts} />
    </div>
  );
}

function ResourceRow({ resource }: { resource: AuthorizationResource }) {
  const [assignmentsOpen, setAssignmentsOpen] = useState(false);
  const resourceHref = `/admin/resources/${encodeURIComponent(resource.type)}/${encodeURIComponent(resource.external_id)}`;
  return (
    <>
      <tr className="border-b border-comb-light align-top hover:bg-glass-light">
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
            ? resource.direct_admins?.map((admin) => <div key={admin} className="text-[10px]" title={admin}>{admin}</div>)
            : <span className="text-muted-foreground">None</span>}
        </td>
        <td className="px-3 py-2 text-right tabular-nums">
          {resource.assignment_count > 0 ? (
            <button
              type="button"
              className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-[11px] font-medium hover:bg-honey/10 hover:text-honey-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-expanded={assignmentsOpen}
              aria-label={`${assignmentsOpen ? "Hide" : "Show"} assignments for ${resource.name}`}
              onClick={() => setAssignmentsOpen((open) => !open)}
            >
              {resource.assignment_count}
              <ChevronDown className={cn("size-3 transition-transform", assignmentsOpen && "rotate-180")} />
            </button>
          ) : "0"}
        </td>
        <td className="px-3 py-2.5 text-muted-foreground">{formatDateTime(resource.created_at)}</td>
        <td className="px-3 py-2.5">
          <span className={cn(
            "rounded-full px-1.5 py-0.5 text-[10px] font-medium",
            resource.last_error ? "bg-destructive/10 text-destructive" : "bg-green-500/10 text-green-700 dark:text-green-400",
          )}>{resource.last_error ? "error" : resource.sync_state}</span>
          {resource.last_error && <div className="mt-1 max-w-52 text-[10px] text-destructive" title={resource.last_error}>{resource.last_error}</div>}
        </td>
      </tr>
      {assignmentsOpen && (
        <tr className="border-b border-comb-light bg-glass-light/50">
          <td colSpan={8} className="px-3 py-3">
            <div className="ml-auto max-w-3xl rounded-md border border-glass-border-honey bg-background/70 p-2">
              <div className="px-2 pb-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Direct resource assignments</div>
              <div className="divide-y divide-comb-light">
                {resource.assignments?.map((assignment) => {
                  const SubjectIcon = assignment.subject_type === "group" ? UsersRound : UserRound;
                  return (
                    <div key={`${assignment.subject_type}:${assignment.subject_id}:${assignment.role}`} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-2 py-2">
                      <div className="flex min-w-0 items-center gap-2.5">
                        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-honey/10 text-honey-dark"><SubjectIcon className="size-3.5" /></span>
                        <div className="min-w-0">
                          <div className="truncate text-xs font-medium">{assignment.subject_label}</div>
                          <div className="truncate font-mono text-[9px] text-muted-foreground" title={assignment.subject_id}>{assignment.subject_id}</div>
                        </div>
                      </div>
                      <div className="text-right">
                        <div className="font-mono text-[10px] font-medium">{assignment.role}</div>
                        <div className="mt-0.5 text-[9px] capitalize text-muted-foreground">
                          {assignment.subject_type === "group" ? "Group assignment" : `${assignment.source} assignment`}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

function ResetDialog({
  enabled,
  accounts,
  operations,
  activeOperation,
  startReset,
}: {
  enabled: boolean;
  accounts: AccountOption[];
  operations: AuthorizationOperation[];
  activeOperation?: AuthorizationOperation;
  startReset: ReturnType<typeof useStartAuthorizationResourceReset>;
}) {
  const [open, setOpen] = useState(false);
  const [accountID, setAccountID] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const selectedAccount = accounts.find((current) => current.id === accountID);
  const latestDryRun = operations.find(
    (operation) => operation.account_id === accountID && operation.dry_run && operation.status === "succeeded",
  );
  const count = latestDryRun?.target_count;
  return (
    <AlertDialog open={open} onOpenChange={(next) => {
      setOpen(next);
      if (!next) {
        setAccountID("");
        setConfirmation("");
      }
    }}>
      <AlertDialogTrigger asChild>
        <Button variant="destructive" size="sm" disabled={!enabled || accounts.length === 0 || Boolean(activeOperation)} title={!enabled ? "Preview reset is disabled by server configuration" : undefined}>
          <Trash2 className="size-3.5" /> Reset FGA resources
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Reset WorkOS authorization resources for one account?</AlertDialogTitle>
          <AlertDialogDescription>
            Queen deletes product resources only inside the selected account's WorkOS organization. The WorkOS organization root is never targeted.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3">
          <label className="block space-y-1.5 text-xs">
            <span>Account</span>
            <select
              className="h-9 w-full rounded-md border bg-background px-2 text-xs"
              value={accountID}
              onChange={(event) => {
                setAccountID(event.target.value);
                setConfirmation("");
              }}
            >
              <option value="">Select an account</option>
              {accounts.map((current) => <option key={current.id} value={current.id}>{current.name}</option>)}
            </select>
          </label>
          <div className="rounded-md border bg-muted/30 p-3 text-xs">
            {latestDryRun ? (
              <p>Latest dry run for <strong>{selectedAccount?.name}</strong> found <strong>{latestDryRun.target_count}</strong> resources.</p>
            ) : (
              <p>Choose an account, then run a dry run to capture its exact deletion target.</p>
            )}
            <Button className="mt-2" variant="outline" size="xs" disabled={!accountID || startReset.isPending || Boolean(activeOperation)} onClick={() => startReset.mutate({ account_id: accountID, dry_run: true })}>
              Run dry run
            </Button>
          </div>
          {latestDryRun && selectedAccount && (
            <label className="block space-y-1.5 text-xs">
              <span>Type <strong>{selectedAccount.name}</strong> to confirm this account.</span>
              <Input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} placeholder={selectedAccount.name} />
            </label>
          )}
          {startReset.error && <p className="text-xs text-destructive">{mutationErrorMessage(startReset.error)}</p>}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={!selectedAccount || count == null || confirmation !== selectedAccount.name || startReset.isPending || Boolean(activeOperation)}
            onClick={() => startReset.mutate(
              { account_id: selectedAccount?.id ?? "", dry_run: false, confirmed_count: count },
              { onSuccess: () => setOpen(false) },
            )}
          >
            Reset {selectedAccount?.name ?? "account"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function Operations({ operations, accounts }: { operations: AuthorizationOperation[]; accounts: AccountOption[] }) {
  if (operations.length === 0) return null;
  return (
    <div className="rounded-lg glass p-3">
      <h3 className="text-xs font-semibold">Recent reset operations</h3>
      <div className="mt-2 space-y-1.5">
        {operations.slice(0, 5).map((operation) => (
          <div key={operation.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
            <span className="font-mono text-muted-foreground">{truncateUUID(operation.id)}</span>
            <span>{operation.dry_run ? "Dry run" : "Reset"}</span>
            <span>{accountName(accounts, operation.account_id)}</span>
            <span className="font-medium">{operation.status}</span>
            <span className="text-muted-foreground">{operation.processed_count}/{operation.target_count} processed</span>
            {!operation.dry_run && <span className="text-muted-foreground">{operation.succeeded_count} deleted · {operation.failed_count} failed</span>}
            {operation.last_error && <span className="text-destructive">{operation.last_error}</span>}
            <span className="ml-auto text-muted-foreground">{formatDateTime(operation.created_at)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function Filter({ value, onChange, label, options }: { value: string; onChange: (value: string) => void; label: string; options: Array<{ value: string; label: string }> }) {
  return (
    <select className="h-9 rounded-md border bg-background px-2 text-xs" value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="all">{label}</option>
      {options.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
    </select>
  );
}

function accountName(accounts: AccountOption[], accountID: string) {
  return accounts.find((current) => current.id === accountID)?.name ?? truncateUUID(accountID);
}

function Stat({ icon: Icon, value, label, warn }: { icon: typeof ShieldCheck; value: number; label: string; warn?: boolean }) {
  return (
    <div className="flex items-center gap-3 rounded-lg glass px-3 py-2.5">
      <span className={cn("flex size-8 items-center justify-center rounded-md bg-honey/10 text-honey-dark", warn && "bg-destructive/10 text-destructive")}>
        <Icon className="size-3.5" />
      </span>
      <div><div className="text-lg font-semibold tabular-nums">{value}</div><div className="text-[10px] text-muted-foreground">{label}</div></div>
    </div>
  );
}
