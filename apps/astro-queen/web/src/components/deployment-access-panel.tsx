import { useMemo, useState } from "react";
import {
  Building2,
  CircleSlash,
  GitBranch,
  KeyRound,
  RefreshCw,
  Search,
  ShieldCheck,
  UserRoundCheck,
  UsersRound,
} from "lucide-react";

import { useDeploymentAccess } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, truncateUUID } from "@/lib/utils";
import type { AdminDeploymentAccessMember } from "@/types/admin";

interface DeploymentAccessPanelProps {
  deploymentId: string;
  active: boolean;
}

const sourceDetails = {
  organization: { label: "Organization", icon: Building2 },
  direct: { label: "Direct", icon: UserRoundCheck },
  group: { label: "Group", icon: UsersRound },
} as const;

export function DeploymentAccessPanel({ deploymentId, active }: DeploymentAccessPanelProps) {
  const query = useDeploymentAccess(deploymentId, active);
  const [search, setSearch] = useState("");
  const members = query.data?.members ?? [];
  const filteredMembers = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (!needle) return members;
    return members.filter((member) =>
      [
        member.email,
        member.user_id,
        ...(member.organization_roles ?? []),
        ...(member.deployment_roles ?? []),
        ...(member.permissions ?? []),
        ...(member.sources ?? []),
      ].some((value) => value?.toLowerCase().includes(needle))
    );
  }, [members, search]);

  if (query.isLoading) {
    return <Skeleton className="h-64 w-full" />;
  }
  if (query.error) {
    return (
      <AccessState
        icon={CircleSlash}
        title="Access evidence could not be loaded"
        detail={query.error.message}
        action={<Button variant="outline" size="xs" onClick={() => query.refetch()}>Try again</Button>}
      />
    );
  }
  if (!query.data) return null;
  if (query.data.status !== "available") {
    const title = query.data.status === "personal"
      ? "Personal deployment"
      : query.data.status === "not_registered"
        ? "Access resource not registered"
        : "FGA inspection is unavailable";
    return (
      <AccessState
        icon={query.data.status === "personal" ? UserRoundCheck : CircleSlash}
        title={title}
        detail={query.data.message ?? "No fine-grained access evidence is available for this deployment."}
      />
    );
  }

  const accessible = members.filter((member) => (member.permissions?.length ?? 0) > 0).length;
  const inherited = members.filter((member) => member.sources?.includes("organization")).length;
  const groupDerived = members.filter((member) => member.sources?.includes("group")).length;

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg glass">
        <div className="h-0.5 honey-gradient" />
        <div className="flex flex-wrap items-start justify-between gap-3 p-4">
          <div className="flex items-start gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-honey/10 text-honey-dark">
              <ShieldCheck className="size-4.5" />
            </span>
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-semibold">Fine-grained access</h3>
                <span className="rounded-full bg-green-500/10 px-2 py-0.5 text-[10px] font-medium text-green-700">
                  Live from WorkOS
                </span>
              </div>
              <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
                Effective permissions for every organization membership, plus the role and path that grants them.
              </p>
            </div>
          </div>
          <Button variant="outline" size="xs" onClick={() => query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={cn("size-3", query.isFetching && "animate-spin")} />
            Refresh evidence
          </Button>
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-3">
        <AccessStat icon={KeyRound} value={accessible} label="members with access" />
        <AccessStat icon={Building2} value={inherited} label="organization-inherited" />
        <AccessStat icon={GitBranch} value={groupDerived} label="group-derived" />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="relative block min-w-64 flex-1 sm:max-w-sm">
          <span className="sr-only">Filter members and permissions</span>
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter by member, role, permission, or source"
            className="pl-7"
          />
        </label>
        <p className="text-[11px] text-muted-foreground">
          Showing {filteredMembers.length} of {members.length} memberships
        </p>
      </div>

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full min-w-[940px] text-[11px]">
          <thead className="glass-subtle">
            <tr className="border-b border-glass-border-honey text-left text-muted-foreground">
              <th className="px-3 py-2 font-medium">Member</th>
              <th className="px-3 py-2 font-medium">Organization role</th>
              <th className="px-3 py-2 font-medium">Deployment role</th>
              <th className="px-3 py-2 font-medium">Effective permissions</th>
              <th className="px-3 py-2 font-medium">Granted through</th>
            </tr>
          </thead>
          <tbody>
            {filteredMembers.map((member) => (
              <AccessMemberRow key={member.user_id} member={member} />
            ))}
          </tbody>
        </table>
        {filteredMembers.length === 0 && (
          <div className="px-4 py-10 text-center text-xs text-muted-foreground">
            No memberships match this filter.
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-1.5 text-[10px] text-muted-foreground">
        <span className="mr-1 pt-0.5">Permission catalog</span>
        {(query.data.permissions ?? []).map((permission) => (
          <PermissionBadge key={permission} permission={permission} muted />
        ))}
      </div>
    </div>
  );
}

function AccessMemberRow({ member }: { member: AdminDeploymentAccessMember }) {
  const permissions = member.permissions ?? [];
  const deploymentRoles = member.deployment_roles ?? [];
  return (
    <tr className="border-b border-comb-light align-top last:border-0 hover:bg-glass-light">
      <td className="px-3 py-2.5">
        <div className="font-medium">{member.email || "Email unavailable"}</div>
        <div className="mt-0.5 flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          {truncateUUID(member.user_id)}
          {member.membership_status !== "active" && (
            <span className="rounded-full bg-amber-100 px-1.5 py-0.5 font-sans text-[9px] text-amber-700">
              {member.membership_status}
            </span>
          )}
        </div>
      </td>
      <td className="px-3 py-2.5">
        <BadgeList values={member.organization_roles ?? []} tone="organization" empty="No org role" />
      </td>
      <td className="px-3 py-2.5">
        <BadgeList values={deploymentRoles} tone="deployment" empty="No deployment role" />
      </td>
      <td className="px-3 py-2.5">
        {permissions.length > 0 ? (
          <div className="flex max-w-md flex-wrap gap-1">
            {permissions.map((permission) => <PermissionBadge key={permission} permission={permission} />)}
          </div>
        ) : (
          <span className="inline-flex items-center gap-1 text-muted-foreground">
            <CircleSlash className="size-3" /> No deployment access
          </span>
        )}
      </td>
      <td className="px-3 py-2.5">
        <div className="flex flex-wrap gap-1">
          {(member.sources ?? []).map((source) => {
            const detail = sourceDetails[source];
            const Icon = detail.icon;
            return (
              <span key={source} className="inline-flex items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                <Icon className="size-2.5" /> {detail.label}
              </span>
            );
          })}
          {(member.sources?.length ?? 0) === 0 && <span className="text-muted-foreground">—</span>}
        </div>
      </td>
    </tr>
  );
}

function BadgeList({ values, tone, empty }: { values: string[]; tone: "organization" | "deployment"; empty: string }) {
  if (values.length === 0) return <span className="text-muted-foreground">{empty}</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {values.map((value) => (
        <span
          key={value}
          className={cn(
            "rounded-full px-1.5 py-0.5 text-[10px] font-medium",
            tone === "organization" ? "bg-teal/10 text-teal" : "bg-honey/10 text-honey-dark"
          )}
        >
          {roleLabel(value)}
        </span>
      ))}
    </div>
  );
}

function PermissionBadge({ permission, muted = false }: { permission: string; muted?: boolean }) {
  return (
    <span
      title={permission}
      className={cn(
        "rounded border px-1.5 py-0.5 font-mono text-[9px]",
        muted
          ? "border-border bg-muted/50 text-muted-foreground"
          : "border-honey/20 bg-pollen-light text-honey-dark"
      )}
    >
      {permissionLabel(permission)}
    </span>
  );
}

function AccessStat({ icon: Icon, value, label }: { icon: typeof KeyRound; value: number; label: string }) {
  return (
    <div className="flex items-center gap-2.5 rounded-lg glass px-3 py-2.5">
      <Icon className="size-3.5 text-honey" />
      <div>
        <div className="text-sm font-semibold tabular-nums">{value}</div>
        <div className="text-[10px] text-muted-foreground">{label}</div>
      </div>
    </div>
  );
}

function AccessState({ icon: Icon, title, detail, action }: { icon: typeof CircleSlash; title: string; detail: string; action?: React.ReactNode }) {
  return (
    <div className="flex min-h-48 items-center justify-center rounded-lg glass p-8 text-center">
      <div className="max-w-md">
        <Icon className="mx-auto size-6 text-muted-foreground" />
        <h3 className="mt-3 text-sm font-medium">{title}</h3>
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
        {action && <div className="mt-3">{action}</div>}
      </div>
    </div>
  );
}

function roleLabel(role: string) {
  return role
    .replace(/^deployment-/, "")
    .split(/[-_]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function permissionLabel(permission: string) {
  const parts = permission.split(":");
  return (parts[parts.length - 1] ?? permission).replace(/_/g, " ");
}
