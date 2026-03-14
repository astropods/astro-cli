import { useState } from "react";
import {
  useQuotaRequests,
  useApproveQuotaRequest,
  useDenyQuotaRequest,
} from "@/api/admin";
import type { QuotaRequest } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Check, X } from "lucide-react";
import { formatDateTime } from "@/lib/utils";

const FEATURE_LABELS: Record<string, string> = {
  compute: "Compute",
  agent_builds: "Agent Builds",
  agent_deployments: "Deployments",
  agents: "Agents",
  members: "Members",
};

const STATUS_STYLES: Record<string, string> = {
  pending: "bg-amber-500/10 text-amber-600",
  approved: "bg-green-500/10 text-green-600",
  denied: "bg-red-500/10 text-red-500",
};

export function QuotaRequestsPage() {
  const [filter, setFilter] = useState<string>("pending");
  const { data, isLoading, error } = useQuotaRequests(filter === "all" ? undefined : filter);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Quota Increase Requests</h2>
          <p className="text-[10px] text-muted-foreground">
            Review and approve quota increase requests from users.
          </p>
        </div>
        <Select value={filter} onValueChange={setFilter}>
          <SelectTrigger className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pending">Pending</SelectItem>
            <SelectItem value="approved">Approved</SelectItem>
            <SelectItem value="denied">Denied</SelectItem>
            <SelectItem value="all">All</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full text-[11px] whitespace-nowrap">
          <thead>
            <tr className="border-b border-glass-border-honey glass-subtle">
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Status</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Account</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Feature</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Usage</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Quota</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Requested</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Reason</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Date</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Granted</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Actions</th>
            </tr>
          </thead>
          <tbody>
            {data?.requests?.length === 0 && (
              <tr>
                <td colSpan={10} className="px-2 py-4 text-center text-muted-foreground">
                  No {filter === "all" ? "" : filter} requests.
                </td>
              </tr>
            )}
            {data?.requests?.map((req) => (
              <RequestRow key={req.id} request={req} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function RequestRow({ request: req }: { request: QuotaRequest }) {
  const approveMut = useApproveQuotaRequest();
  const denyMut = useDenyQuotaRequest();
  const [editing, setEditing] = useState(false);
  const [grantAmount, setGrantAmount] = useState(
    req.requested_amount > 0 ? String(req.requested_amount) : ""
  );
  const [note, setNote] = useState("");

  const handleApprove = () => {
    const amount = parseFloat(grantAmount);
    if (!amount || amount <= 0) return;
    approveMut.mutate(
      { id: req.id, grantAmount: amount, note },
      { onSuccess: () => setEditing(false) }
    );
  };

  const handleDeny = () => {
    denyMut.mutate(
      { id: req.id, note },
      { onSuccess: () => setEditing(false) }
    );
  };

  return (
    <>
      <tr className="border-b border-glass-border-honey hover:bg-glass-light">
        <td className="px-2 py-1.5">
          <span className={`inline-block rounded-full px-2 py-0.5 text-[10px] font-medium ${STATUS_STYLES[req.status] ?? "bg-muted text-muted-foreground"}`}>
            {req.status}
          </span>
        </td>
        <td className="px-2 py-1.5">{req.account_name || req.account_id}</td>
        <td className="px-2 py-1.5 font-medium">{FEATURE_LABELS[req.feature_key] ?? req.feature_key}</td>
        <td className="px-2 py-1.5 text-right tabular-nums">{req.current_usage}</td>
        <td className="px-2 py-1.5 text-right tabular-nums">{req.current_quota || "—"}</td>
        <td className="px-2 py-1.5 text-right tabular-nums">{req.requested_amount || "—"}</td>
        <td className="px-2 py-1.5 text-muted-foreground max-w-[200px] truncate" title={req.reason}>
          {req.reason || "—"}
        </td>
        <td className="px-2 py-1.5 text-muted-foreground">{formatDateTime(req.created_at)}</td>
        <td className="px-2 py-1.5 text-right tabular-nums">
          {req.grant_amount > 0 ? (
            <span className="text-green-600">{req.grant_amount}</span>
          ) : "—"}
        </td>
        <td className="px-2 py-1.5">
          {req.status === "pending" && !editing && (
            <div className="flex gap-1">
              <Button variant="ghost" size="icon-xs" title="Approve" onClick={() => setEditing(true)}>
                <Check className="size-3 text-green-600" />
              </Button>
              <Button variant="ghost" size="icon-xs" title="Deny" onClick={handleDeny} disabled={denyMut.isPending}>
                <X className="size-3 text-red-500" />
              </Button>
            </div>
          )}
          {req.status !== "pending" && req.resolution_note && (
            <span className="text-[10px] text-muted-foreground" title={req.resolution_note}>
              {req.resolution_note}
            </span>
          )}
        </td>
      </tr>
      {editing && (
        <tr className="border-b border-glass-border-honey bg-glass-light">
          <td colSpan={10} className="px-2 py-2">
            <div className="flex items-end gap-2">
              <div>
                <label className="text-[10px] font-medium">Grant amount *</label>
                <Input
                  type="number"
                  min={0}
                  value={grantAmount}
                  onChange={(e) => setGrantAmount(e.target.value)}
                  placeholder="Amount"
                  className="mt-0.5 w-28"
                />
              </div>
              <div className="flex-1">
                <label className="text-[10px] font-medium">Note</label>
                <Input
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Optional note"
                  className="mt-0.5"
                />
              </div>
              <Button
                size="xs"
                onClick={handleApprove}
                disabled={!grantAmount || parseFloat(grantAmount) <= 0 || approveMut.isPending}
              >
                {approveMut.isPending ? "..." : "Approve"}
              </Button>
              <Button size="xs" variant="outline" onClick={handleDeny} disabled={denyMut.isPending}>
                {denyMut.isPending ? "..." : "Deny"}
              </Button>
              <Button size="xs" variant="ghost" onClick={() => setEditing(false)}>
                Cancel
              </Button>
            </div>
            {approveMut.error && <p className="text-[10px] text-destructive mt-1">{approveMut.error.message}</p>}
            {denyMut.error && <p className="text-[10px] text-destructive mt-1">{denyMut.error.message}</p>}
          </td>
        </tr>
      )}
    </>
  );
}
