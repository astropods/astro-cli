import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router";
import {
  useAccount,
  useAccountMetronomeAliases,
  useRecoverAccountMetronomeAliases,
  useRegisterAccountMetronome,
  useRecoverAccountLangfuse,
  useRecoverAccountBifrost,
  useClusters,
  useRenameAccount,
  useSetAccountCluster,
  useInvalidateAccountCaches,
  useQuotaRequests,
  useApproveQuotaRequest,
  useDenyQuotaRequest,
} from "@/api/admin";
import type { QuotaRequest } from "@/api/admin";
import type { AccountBillingInfo, AccountResourceLimit, AccountMemberInfo } from "@/types/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  ArrowLeft,
  Check,
  X,
  Copy,
  Trash2,
  Crown,
  CircleCheck,
  AlertTriangle,
} from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import { FEATURE_LABELS } from "@/pages/quota-requests";

const PRIMARY_CLUSTER_VALUE = "__primary__";

const BILLING_STATUS_STYLES: Record<string, string> = {
  active: "bg-green-500/10 text-green-600",
  past_due: "bg-amber-500/10 text-amber-600",
  suspended: "bg-red-500/10 text-red-500",
};

function formatLimit(limit: number): string {
  if (limit < 0) return "Unlimited";
  if (limit === 0) return "Disabled";
  return String(limit);
}

export function AccountDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error } = useAccount(id);

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-destructive">Error: {error.message}</p>;
  if (!data) return null;

  const { account, billing, limits, members } = data;
  const isDeleted = !!account.deleted_at;
  const langfuseProjectId = data.langfuse_project_id ?? "";

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/admin/accounts"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="size-3" /> Accounts
        </Link>
      </div>

      <AccountHeader account={account} isDeleted={isDeleted} />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <BillingCard billing={billing} accountId={account.id} />
        <PlacementCard accountId={account.id} clusterId={account.cluster_id ?? ""} disabled={isDeleted} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <LimitsCard limits={limits ?? []} />
        <ObservabilityCard
          accountId={account.id}
          hasLangfuse={account.has_langfuse}
          langfuseProjectId={langfuseProjectId}
          bifrostCustomerId={billing?.bifrost_customer_id ?? ""}
        />
      </div>

      <AccountQuotaRequests accountId={account.id} />

      <MembersCard members={members ?? []} />

      {!isDeleted && (
        <DangerCard
          accountId={account.id}
          accountName={account.name}
          onDeleted={() => navigate("/admin/accounts")}
        />
      )}
    </div>
  );
}

function Section({ title, children, action }: { title: string; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="rounded-lg glass p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">{title}</h3>
        {action}
      </div>
      {children}
    </div>
  );
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      variant="ghost"
      size="icon-xs"
      title="Copy"
      onClick={() => {
        void navigator.clipboard.writeText(value);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? <Check className="size-3 text-green-600" /> : <Copy className="size-3" />}
    </Button>
  );
}

function AccountHeader({
  account,
  isDeleted,
}: {
  account: { id: string; name: string; type: string; created_at: string; updated_at: string; deleted_at?: string };
  isDeleted: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <div className="flex size-11 shrink-0 items-center justify-center rounded-lg bg-pollen/60 text-lg font-bold text-honey-dark">
          {account.name.charAt(0).toUpperCase()}
        </div>
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-semibold">{account.name}</h2>
            {isDeleted ? (
              <span className="inline-flex items-center rounded-full bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive">
                Deleted {formatDateTime(account.deleted_at!)}
              </span>
            ) : (
              <span className="inline-flex items-center rounded-full bg-green-500/10 px-1.5 py-0.5 text-[10px] font-medium text-green-600">
                Active
              </span>
            )}
          </div>
          <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
            <span className="rounded bg-glass-light px-1.5 py-0.5">{account.type}</span>
            <span className="font-mono">{account.id}</span>
            <CopyButton value={account.id} />
          </div>
          <p className="mt-1 text-[11px] text-muted-foreground">
            Created {formatDateTime(account.created_at)} · Updated {formatDateTime(account.updated_at)}
          </p>
        </div>
      </div>
    </div>
  );
}

function BillingCard({ billing, accountId }: { billing?: AccountBillingInfo; accountId: string }) {
  const status = billing?.status || "";
  const metronomeId = billing?.metronome_customer_id ?? "";
  const registerMut = useRegisterAccountMetronome();
  return (
    <Section title="Billing">
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          {status ? (
            <span className={`inline-block rounded-full px-2 py-0.5 text-[11px] font-medium ${BILLING_STATUS_STYLES[status] ?? "bg-muted text-muted-foreground"}`}>
              {status.replace("_", " ")}
            </span>
          ) : (
            <span className="inline-block rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
              Never billed
            </span>
          )}
          {billing?.alert_active && (
            <span className="inline-block rounded-full bg-red-500/10 px-2 py-0.5 text-[10px] font-medium text-red-500">
              Alert active
            </span>
          )}
        </div>
        {billing?.reason && (
          <p className="text-xs text-muted-foreground">Reason: {billing.reason}</p>
        )}
        {billing?.dunning_since && (
          <p className="text-xs text-muted-foreground">In dunning since {formatDateTime(billing.dunning_since)}</p>
        )}
        {billing?.updated_at && (
          <p className="text-xs text-muted-foreground">Updated {formatDateTime(billing.updated_at)}</p>
        )}
        <div className="space-y-2 pt-1">
          <ObservabilityRow
            label="Metronome"
            value={metronomeId}
            actionLabel="Register"
            onRecover={() => registerMut.mutate(accountId)}
            pending={registerMut.isPending}
            error={registerMut.error}
          />
          <IdRow label="Stripe" value={billing?.stripe_customer_id ?? ""} />
        </div>
        {metronomeId && <MetronomeAliasCheck accountId={accountId} />}
      </div>
    </Section>
  );
}

function AliasTable({ expected, actual }: { expected: string[]; actual: string[] }) {
  const all = Array.from(new Set([...expected, ...actual]));
  if (all.length === 0) {
    return <p className="text-[10px] text-muted-foreground">No aliases set.</p>;
  }
  return (
    <table className="w-full text-[10px]">
      <thead>
        <tr className="text-muted-foreground">
          <th className="py-0.5 text-left font-medium">Alias</th>
          <th className="py-0.5 text-center font-medium">Expected</th>
          <th className="py-0.5 text-center font-medium">Present</th>
        </tr>
      </thead>
      <tbody>
        {all.map((alias) => {
          const isExpected = expected.includes(alias);
          const isPresent = actual.includes(alias);
          const missing = isExpected && !isPresent;
          return (
            <tr key={alias} className={missing ? "text-red-500" : ""}>
              <td className="py-0.5 font-mono break-all">{alias}</td>
              <td className="py-0.5 text-center">{isExpected ? "✓" : "—"}</td>
              <td className="py-0.5 text-center">{isPresent ? "✓" : "—"}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function MetronomeAliasCheck({ accountId }: { accountId: string }) {
  const { data, isLoading, error } = useAccountMetronomeAliases(accountId, true);
  const recoverMut = useRecoverAccountMetronomeAliases();

  const canRecover = !!data && data.configured && !data.error && !data.ok;

  const badge =
    data && data.configured && !data.error ? (
      data.ok ? (
        <span className="inline-flex items-center gap-1 text-green-600">
          <CircleCheck className="size-3.5" /> OK
        </span>
      ) : (
        <span className="inline-flex items-center gap-1 text-red-500">
          <AlertTriangle className="size-3.5" /> Incomplete
        </span>
      )
    ) : null;

  return (
    <div className="rounded bg-glass-light px-2 py-1.5">
      <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium">
        Metronome ingest aliases
        {badge}
      </div>
      {isLoading && <p className="text-[10px] text-muted-foreground">Checking against Metronome…</p>}
      {error && <p className="text-[10px] text-destructive">Check failed: {error.message}</p>}
      {data && !isLoading && (
        data.error ? (
          <p className="text-[10px] text-destructive">Check failed: {data.error}</p>
        ) : !data.configured ? (
          <p className="text-[10px] text-muted-foreground">
            Billing provider unavailable on this server — cannot verify aliases.
          </p>
        ) : (
          <AliasTable expected={data.expected ?? []} actual={data.actual ?? []} />
        )
      )}
      {canRecover && (
        <div className="mt-1.5">
          <Button
            size="xs"
            variant="outline"
            disabled={recoverMut.isPending}
            onClick={() => recoverMut.mutate(accountId)}
          >
            {recoverMut.isPending ? "Recovering…" : "Recover aliases"}
          </Button>
          {recoverMut.error && (
            <p className="mt-1 text-[10px] text-destructive">
              Recovery failed: {(recoverMut.error as Error).message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function IdRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
      {value ? (
        <>
          <span className="font-mono break-all">{value}</span>
          <CopyButton value={value} />
        </>
      ) : (
        <span className="text-muted-foreground/40">—</span>
      )}
    </div>
  );
}

function PlacementCard({ accountId, clusterId, disabled }: { accountId: string; clusterId: string; disabled: boolean }) {
  const { data: clustersData } = useClusters(true);
  const setClusterMut = useSetAccountCluster();
  const additionalClusters = (clustersData?.clusters ?? []).filter((c) => !c.is_primary);
  const [pending, setPending] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const savedValue = clusterId === "" ? PRIMARY_CLUSTER_VALUE : clusterId;
  const effective = pending ?? savedValue;
  const dirty = effective !== savedValue;

  const migrate = () => {
    const target = effective === PRIMARY_CLUSTER_VALUE ? "" : effective;
    setClusterMut.mutate(
      { id: accountId, clusterId: target },
      {
        onSuccess: (resp) => {
          setPending(null);
          const count = resp.migrations_enqueued ?? 0;
          setMessage(count > 0
            ? `${count} deployment migration${count === 1 ? "" : "s"} queued. Track in Admin → Migrations.`
            : "Cluster updated; no deployment migrations queued.");
        },
        onError: (e) => setMessage(`Cluster change failed: ${(e as Error).message}`),
      },
    );
  };

  return (
    <Section title="Cluster placement">
      <div className="flex items-center gap-2">
        <Select value={effective} onValueChange={(v) => { setPending(v); setMessage(null); }} disabled={disabled || setClusterMut.isPending}>
          <SelectTrigger className="h-7 w-56 text-xs">
            <SelectValue placeholder="Primary (default)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={PRIMARY_CLUSTER_VALUE}>Primary (default)</SelectItem>
            {additionalClusters.map((c) => (
              <SelectItem key={c.id} value={c.id}>{c.id}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {dirty && (
          <>
            <Button size="sm" className="h-7 px-2 text-xs" disabled={setClusterMut.isPending} onClick={migrate}>
              {setClusterMut.isPending ? "Migrating…" : "Migrate"}
            </Button>
            <Button variant="ghost" size="icon-xs" onClick={() => setPending(null)} title="Cancel">
              <X className="size-3" />
            </Button>
          </>
        )}
      </div>
      {message && <p className="mt-2 text-[11px] text-muted-foreground">{message}</p>}
    </Section>
  );
}

function ObservabilityCard({ accountId, hasLangfuse, langfuseProjectId, bifrostCustomerId }: { accountId: string; hasLangfuse: boolean; langfuseProjectId: string; bifrostCustomerId: string }) {
  const recoverLangfuse = useRecoverAccountLangfuse();
  const recoverBifrost = useRecoverAccountBifrost();
  const langfuseValue = langfuseProjectId || (hasLangfuse ? "connected" : "");

  return (
    <Section title="Observability">
      <div className="space-y-2">
        <ObservabilityRow
          label="Langfuse"
          value={langfuseValue}
          onRecover={() => recoverLangfuse.mutate(accountId)}
          pending={recoverLangfuse.isPending}
          error={recoverLangfuse.error}
        />
        <ObservabilityRow
          label="Bifrost"
          value={bifrostCustomerId}
          onRecover={() => recoverBifrost.mutate(accountId)}
          pending={recoverBifrost.isPending}
          error={recoverBifrost.error}
        />
      </div>
    </Section>
  );
}

function ObservabilityRow({ label, value, onRecover, pending, error, actionLabel = "Recover" }: {
  label: string;
  value: string;
  onRecover: () => void;
  pending: boolean;
  error: Error | null;
  actionLabel?: string;
}) {
  return (
    <div>
      <div className="flex items-center gap-2 text-xs">
        <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
        {value ? (
          <>
            <span className="font-mono break-all">{value}</span>
            <CopyButton value={value} />
          </>
        ) : (
          <>
            <span className="text-muted-foreground/40">—</span>
            <Button size="xs" variant="outline" className="ml-auto" disabled={pending} onClick={onRecover}>
              {pending ? `${actionLabel}ing…` : actionLabel}
            </Button>
          </>
        )}
      </div>
      {error && <p className="mt-0.5 text-[10px] text-destructive">{actionLabel} failed: {error.message}</p>}
    </div>
  );
}

function LimitsCard({ limits }: { limits: AccountResourceLimit[] }) {
  return (
    <Section title="Resource usage & limits">
      {limits.length === 0 ? (
        <p className="text-xs text-muted-foreground">No usage data available.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-1 text-left font-medium text-muted-foreground">Resource</th>
                <th className="px-2 py-1 text-right font-medium text-muted-foreground">Used</th>
                <th className="px-2 py-1 text-right font-medium text-muted-foreground">Limit</th>
              </tr>
            </thead>
            <tbody>
              {limits.map((l) => {
                const over = l.limit > 0 && l.used >= l.limit;
                return (
                  <tr key={l.resource} className="border-b border-comb-light">
                    <td className="px-2 py-1">{FEATURE_LABELS[l.resource] ?? l.resource}</td>
                    <td className={`px-2 py-1 text-right tabular-nums ${over ? "text-red-500" : ""}`}>{l.used}</td>
                    <td className="px-2 py-1 text-right tabular-nums text-muted-foreground">{formatLimit(l.limit)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}

function MembersCard({ members }: { members: AccountMemberInfo[] }) {
  return (
    <Section title={`Members (${members.length})`}>
      {members.length === 0 ? (
        <p className="text-xs text-muted-foreground">No members.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-1 text-left font-medium text-muted-foreground">Email</th>
                <th className="px-2 py-1 text-left font-medium text-muted-foreground">User ID</th>
                <th className="px-2 py-1 text-left font-medium text-muted-foreground">Joined</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.user_id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-1">
                    <span className="inline-flex items-center gap-1">
                      {m.is_owner && <Crown className="size-3 text-honey-dark" />}
                      {m.email || <span className="text-muted-foreground">—</span>}
                    </span>
                  </td>
                  <td className="px-2 py-1 font-mono text-muted-foreground">{truncateUUID(m.user_id)}</td>
                  <td className="px-2 py-1 text-muted-foreground">{formatDateTime(m.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}

function AccountQuotaRequests({ accountId }: { accountId: string }) {
  const { data } = useQuotaRequests("pending");
  const requests = (data?.requests ?? []).filter((r) => r.account_id === accountId);

  if (requests.length === 0) return null;

  return (
    <Section title={`Pending quota requests (${requests.length})`}>
      <div className="space-y-2">
        {requests.map((req) => (
          <QuotaRequestItem key={req.id} request={req} />
        ))}
      </div>
    </Section>
  );
}

function QuotaRequestItem({ request: req }: { request: QuotaRequest }) {
  const approveMut = useApproveQuotaRequest();
  const denyMut = useDenyQuotaRequest();
  const [grantAmount, setGrantAmount] = useState(req.requested_amount > 0 ? String(req.requested_amount) : "");
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");

  const approve = async () => {
    const amount = parseFloat(grantAmount);
    if (!amount || amount <= 0) return;
    setErr("");
    try {
      await approveMut.mutateAsync({ id: req.id, grantAmount: amount, note });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to approve");
    }
  };

  return (
    <div className="rounded-lg bg-glass-light p-3">
      <div className="flex flex-wrap items-end gap-2">
        <div className="mr-auto">
          <p className="text-xs font-medium">{FEATURE_LABELS[req.feature_key] ?? req.feature_key}</p>
          <p className="text-[10px] text-muted-foreground">
            Using {req.current_usage} of {req.current_quota || "—"} · requested {req.requested_amount || "—"}
          </p>
          {req.reason && <p className="mt-0.5 text-[10px] text-muted-foreground" title={req.reason}>{req.reason}</p>}
        </div>
        <div>
          <label className="text-[10px] font-medium">New limit *</label>
          <Input type="number" min={0} value={grantAmount} onChange={(e) => setGrantAmount(e.target.value)} placeholder="Amount" className="mt-0.5 w-28" />
        </div>
        <div className="min-w-[10rem] flex-1">
          <label className="text-[10px] font-medium">Note</label>
          <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="Optional note" className="mt-0.5" />
        </div>
        <Button size="xs" onClick={approve} disabled={!grantAmount || parseFloat(grantAmount) <= 0 || approveMut.isPending}>
          {approveMut.isPending ? "…" : "Approve"}
        </Button>
        <Button size="xs" variant="outline" onClick={() => denyMut.mutate({ id: req.id, note })} disabled={denyMut.isPending}>
          {denyMut.isPending ? "…" : "Deny"}
        </Button>
      </div>
      {err && <p className="mt-1 text-[10px] text-destructive">{err}</p>}
      {denyMut.error && <p className="mt-1 text-[10px] text-destructive">{denyMut.error.message}</p>}
    </div>
  );
}

function DangerCard({ accountId, accountName, onDeleted }: { accountId: string; accountName: string; onDeleted: () => void }) {
  void onDeleted;
  const invalidateMut = useInvalidateAccountCaches();
  const renameMut = useRenameAccount();
  const [result, setResult] = useState<string | null>(null);
  const [renameOpen, setRenameOpen] = useState(false);
  const [newName, setNewName] = useState(accountName);
  const [renameResult, setRenameResult] = useState<string | null>(null);

  const doRename = () => {
    const next = newName.trim();
    if (!next || next === accountName) return;
    if (!window.confirm(
      `Rename account from "${accountName}" to "${next}"?\n\nThis is destructive: the account name is referenced in URLs and integrations, and renaming can break existing links. Only proceed if you are sure.`,
    )) return;
    renameMut.mutate(
      { id: accountId, newName: next },
      {
        onSuccess: () => { setRenameOpen(false); setRenameResult(`Renamed to "${next}".`); },
        onError: (e) => setRenameResult(`Failed: ${(e as Error).message}`),
      },
    );
  };

  return (
    <Section title="Maintenance">
      <p className="mb-2 text-xs text-muted-foreground">
        Clears this account's server-side caches that back the agents dashboard — the
        agents-page deploy payload, the Insights endpoint cache, and the per-deployment
        observability summaries for active deployments. Use it when the dashboard shows
        stale deployments or metrics; the next page load repopulates from source. It does
        not change any deployment, spec, or account data.
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={invalidateMut.isPending}
          onClick={() => {
            if (!window.confirm(`Invalidate agents-page caches for "${accountName}"?`)) return;
            invalidateMut.mutate(accountId, {
              onSuccess: () => setResult("Caches invalidated."),
              onError: (e) => setResult(`Failed: ${(e as Error).message}`),
            });
          }}
        >
          <Trash2 className="size-3.5" />
          {invalidateMut.isPending ? "Invalidating…" : "Invalidate caches"}
        </Button>
        {result && <span className="text-xs text-muted-foreground">{result}</span>}
      </div>

      <div className="mt-4 border-t border-glass-border-honey pt-3">
        <p className="text-xs font-medium text-red-500">Rename account</p>
        <p className="mb-2 text-[10px] text-muted-foreground">
          Destructive — the account name is referenced in URLs and integrations; renaming can
          break existing links. Avoid unless necessary.
        </p>
        {!renameOpen ? (
          <Button size="xs" variant="outline" onClick={() => { setRenameOpen(true); setNewName(accountName); }}>
            Rename…
          </Button>
        ) : (
          <div className="flex items-center gap-2">
            <Input value={newName} onChange={(e) => setNewName(e.target.value)} className="h-7 w-56" autoFocus />
            <Button
              size="xs"
              variant="destructive"
              disabled={renameMut.isPending || !newName.trim() || newName.trim() === accountName}
              onClick={doRename}
            >
              {renameMut.isPending ? "Renaming…" : "Confirm rename"}
            </Button>
            <Button size="xs" variant="ghost" onClick={() => setRenameOpen(false)}>Cancel</Button>
          </div>
        )}
        {renameResult && <p className="mt-1 text-[10px] text-muted-foreground">{renameResult}</p>}
      </div>
    </Section>
  );
}
