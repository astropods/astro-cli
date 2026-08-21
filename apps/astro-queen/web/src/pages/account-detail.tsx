import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router";
import {
  useAccount,
  useAccountMetronomeAliases,
  useRecoverAccountMetronomeAliases,
  useRegisterAccountMetronome,
  useAccountBilling,
  useRetryBillingProvision,
  useForceBillingResume,
  useRecoverAccountLangfuse,
  useRecoverAccountBifrost,
  useClusters,
  useRenameAccount,
  useAccountClusters,
  useAddAccountCluster,
  useRemoveAccountCluster,
  useSetAccountDefaultCluster,
  useInvalidateAccountCaches,
  useQuotaRequests,
  useApproveQuotaRequest,
  useDenyQuotaRequest,
} from "@/api/admin";
import type { QuotaRequest } from "@/api/admin";
import type {
  AccountBillingInfo,
  AccountResourceLimit,
  AccountMemberInfo,
  BillingContract,
  BillingSpend,
} from "@/types/admin";
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
  ExternalLink,
} from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import { FEATURE_LABELS } from "@/pages/quota-requests";

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
        <PlacementCard accountId={account.id} disabled={isDeleted} />
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

      <BillingOperationsCard accountId={account.id} />

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
  // Same query key as the operations panel, so this costs no extra request.
  const { data: detail } = useAccountBilling(accountId);
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
            href={detail?.metronome_url}
          />
          <IdRow label="Stripe" value={billing?.stripe_customer_id ?? ""} href={detail?.stripe_url} />
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

const COVERAGE_COPY: Record<string, { label: string; tone: string; detail: string }> = {
  covered: {
    label: "On contract",
    tone: "text-green-600",
    detail: "A contract effective now covers this customer, so provisioning creates none.",
  },
  none: {
    label: "No contract",
    tone: "text-amber-600",
    detail: "Nothing covers this customer, so the next sweep creates a contract.",
  },
  unknown: {
    label: "Unknown",
    tone: "text-muted-foreground",
    detail: "The billing provider cannot report contract coverage on this server.",
  },
};

// Only the two actions with no vendor equivalent live here; contracts, credit,
// and refunds stay in the vendor dashboards linked above.
function BillingOperationsCard({ accountId }: { accountId: string }) {
  const { data, isLoading, error } = useAccountBilling(accountId);
  const retryMut = useRetryBillingProvision();
  const resumeMut = useForceBillingResume();
  const [result, setResult] = useState<string | null>(null);

  if (isLoading) return <Section title="Billing operations"><Skeleton className="h-24 w-full" /></Section>;
  if (error) {
    return (
      <Section title="Billing operations">
        <p className="text-xs text-destructive">Failed to load: {error.message}</p>
      </Section>
    );
  }
  if (!data) return null;

  const coverage = COVERAGE_COPY[data.coverage ?? "unknown"] ?? COVERAGE_COPY.unknown;
  const job = data.provision_job;
  const contracts = data.contracts ?? [];

  return (
    <Section
      title="Billing operations"
      action={
        <div className="flex items-center gap-2">
          {data.metronome_url && <VendorLink href={data.metronome_url} label="Metronome" />}
          {data.stripe_url && <VendorLink href={data.stripe_url} label="Stripe" />}
        </div>
      }
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-[11px]">
          <span className={`rounded-full px-2 py-0.5 font-medium ${data.enforced ? "bg-red-500/10 text-red-500" : "bg-muted text-muted-foreground"}`}>
            {data.enforced ? "Enforcing" : "Observe only"}
          </span>
          {data.workloads_suspended && (
            <span className="rounded-full bg-red-500/10 px-2 py-0.5 font-medium text-red-500">
              Workloads suspended
            </span>
          )}
          <span className="text-muted-foreground">
            {data.provisioned_at
              ? `Provisioned ${formatDateTime(data.provisioned_at)}`
              : "Not provisioned"}
          </span>
          {data.card && (
            <span className="text-muted-foreground">
              Card {data.card.brand} ····{data.card.last4} exp {data.card.exp_month}/{data.card.exp_year}
            </span>
          )}
        </div>

        {data.spend && <SpendRow spend={data.spend} />}

        <div className="rounded bg-glass-light px-2 py-1.5">
          <p className={`text-[11px] font-medium ${coverage.tone}`}>{coverage.label}</p>
          <p className="mt-0.5 text-[10px] text-muted-foreground">{coverage.detail}</p>
          {contracts.length > 0 && <ContractTable contracts={contracts} />}
        </div>

        <div className="rounded bg-glass-light px-2 py-1.5">
          <p className="text-[11px] font-medium">Provisioning</p>
          {job ? (
            <p className="mt-0.5 text-[10px] text-muted-foreground">
              Last job {job.state} on attempt {job.attempt}
              {job.finalized_at ? ` at ${formatDateTime(job.finalized_at)}` : ""}
            </p>
          ) : (
            <p className="mt-0.5 text-[10px] text-muted-foreground">
              No recent job. River prunes finished jobs, so this does not mean it never ran.
            </p>
          )}
          {job?.last_error && (
            <p className="mt-0.5 break-all text-[10px] text-destructive">{job.last_error}</p>
          )}
        </div>

        {(data.warnings ?? []).map((warning) => (
          <p key={warning} className="text-[10px] text-amber-600">{warning}</p>
        ))}

        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="xs"
            variant="outline"
            disabled={retryMut.isPending}
            title={
              data.provisioned_at
                ? "Re-run provisioning. Idempotent, and grants credit, which clears a stale credits_exhausted gate."
                : "Re-enqueue the provisioning job"
            }
            onClick={() => {
              setResult(null);
              retryMut.mutate(
                { id: accountId, force: !!data.provisioned_at },
                {
                  onSuccess: (r) =>
                    setResult(
                      r.status === "already_provisioned"
                        ? "Already provisioned; nothing enqueued."
                        : "Provisioning enqueued.",
                    ),
                  onError: (e) => setResult(`Failed: ${(e as Error).message}`),
                },
              );
            }}
          >
            {retryMut.isPending ? "Enqueueing…" : "Retry provisioning"}
          </Button>
          <Button
            size="xs"
            variant="outline"
            disabled={resumeMut.isPending || !data.workloads_suspended}
            title={data.workloads_suspended ? "Restore billing-suspended deployments" : "Nothing is suspended"}
            onClick={() => {
              if (!window.confirm("Restore this account's billing-suspended deployments?\n\nThis does not change billing status, so a still-unpaid account can be suspended again by the next signal.")) return;
              setResult(null);
              resumeMut.mutate(accountId, {
                onSuccess: (r) =>
                  setResult(
                    r.status === "nothing_suspended"
                      ? "Nothing suspended; no job enqueued."
                      : "Resume enqueued.",
                  ),
                onError: (e) => setResult(`Failed: ${(e as Error).message}`),
              });
            }}
          >
            {resumeMut.isPending ? "Resuming…" : "Force resume"}
          </Button>
          {result && <span className="text-[11px] text-muted-foreground">{result}</span>}
        </div>
      </div>
    </Section>
  );
}

// Amounts arrive in the unit named by currency; the server has already
// converted Metronome's own unit, so nothing here rescales money.
function formatAmount(value: number, currency?: string): string {
  const amount = value.toFixed(2);
  return currency === "USD" ? `$${amount}` : `${amount} ${currency ?? ""}`.trim();
}

// Credit remaining is what gating fires on, so it leads and reddens at zero.
function SpendRow({ spend }: { spend: BillingSpend }) {
  const exhausted = spend.has_credit && spend.credit_remaining <= 0;
  return (
    <div className="flex flex-wrap items-baseline gap-x-5 gap-y-1 rounded bg-glass-light px-2 py-1.5">
      <Metric
        label="Credit left"
        value={spend.has_credit ? formatAmount(spend.credit_remaining, spend.currency) : "—"}
        tone={exhausted ? "text-red-500" : "text-green-600"}
      />
      <Metric
        label="This period"
        value={spend.has_current_spend ? formatAmount(spend.current_spend, spend.currency) : "—"}
        hint={spend.current_period_end ? `ends ${formatDateTime(spend.current_period_end)}` : undefined}
      />
      <Metric
        label="Last invoice"
        value={spend.has_last_invoice ? formatAmount(spend.last_invoice_total, spend.currency) : "—"}
        hint={spend.last_invoice_at ? formatDateTime(spend.last_invoice_at) : undefined}
      />
    </div>
  );
}

function Metric({ label, value, tone, hint }: { label: string; value: string; tone?: string; hint?: string }) {
  return (
    <div>
      <p className="text-[10px] text-muted-foreground">{label}</p>
      <p className={`text-sm font-semibold tabular-nums ${tone ?? ""}`}>{value}</p>
      {hint && <p className="text-[10px] text-muted-foreground">{hint}</p>}
    </div>
  );
}

function ContractTable({ contracts }: { contracts: BillingContract[] }) {
  return (
    <table className="mt-1.5 w-full text-[10px]">
      <thead>
        <tr className="text-muted-foreground">
          <th className="py-0.5 text-left font-medium">Contract</th>
          <th className="py-0.5 text-left font-medium">Rate card</th>
          <th className="py-0.5 text-left font-medium">Started</th>
        </tr>
      </thead>
      <tbody>
        {contracts.map((c) => (
          <tr key={c.id}>
            <td className="py-0.5 font-mono break-all">{c.id}</td>
            <td className="py-0.5 font-mono break-all">{c.rate_card_id || "—"}</td>
            <td className="py-0.5">{c.starting_at ? formatDateTime(c.starting_at) : "—"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function VendorLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
    >
      {label} <ExternalLink className="size-3" />
    </a>
  );
}

// IdValue links a provider ID to its dashboard. The URL is resolved server-side
// so it is absent rather than wrong when the environment is unknown.
function IdValue({ value, href }: { value: string; href?: string }) {
  if (!href) return <span className="font-mono break-all">{value}</span>;
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 font-mono break-all underline decoration-dotted underline-offset-2 hover:text-foreground"
    >
      {value}
      <ExternalLink className="size-3 shrink-0" />
    </a>
  );
}

function IdRow({ label, value, href }: { label: string; value: string; href?: string }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
      {value ? (
        <>
          <IdValue value={value} href={href} />
          <CopyButton value={value} />
        </>
      ) : (
        <span className="text-muted-foreground/40">—</span>
      )}
    </div>
  );
}

function PlacementCard({ accountId, disabled }: { accountId: string; disabled: boolean }) {
  const { data: clustersData } = useClusters();
  const { data: allowedData } = useAccountClusters(accountId);
  const addMut = useAddAccountCluster();
  const removeMut = useRemoveAccountCluster();
  const setDefaultMut = useSetAccountDefaultCluster();
  const [toAdd, setToAdd] = useState<string>("");
  const [message, setMessage] = useState<string | null>(null);

  const allowed = allowedData?.clusters ?? [];
  const allowedIds = new Set(allowed.map((c) => c.cluster_id));
  const addable = (clustersData?.clusters ?? []).filter((c) => !allowedIds.has(c.id));
  const busy = addMut.isPending || removeMut.isPending || setDefaultMut.isPending;

  const add = () => {
    if (!toAdd) return;
    addMut.mutate(
      { id: accountId, clusterId: toAdd, setDefault: allowed.length === 0 },
      {
        onSuccess: () => {
          setToAdd("");
          setMessage("Cluster allowed. Deploys can target it once a user picks it.");
        },
        onError: (e) => setMessage(`Could not allow cluster: ${(e as Error).message}`),
      },
    );
  };

  const remove = (clusterId: string) => {
    removeMut.mutate(
      { id: accountId, clusterId },
      {
        onSuccess: () => setMessage(`${clusterId} is no longer allowed for this account.`),
        onError: (e) => setMessage(`Could not remove cluster: ${(e as Error).message}`),
      },
    );
  };

  const makeDefault = (clusterId: string) => {
    setDefaultMut.mutate(
      { id: accountId, clusterId },
      {
        onSuccess: () => setMessage(`${clusterId} is now the default for new deploys.`),
        onError: (e) => setMessage(`Could not set default: ${(e as Error).message}`),
      },
    );
  };

  return (
    <Section title="Cluster placement">
      <div className="space-y-1">
        {allowed.length === 0 && (
          <p className="text-xs text-muted-foreground/70">
            No clusters allowed. New deploys route to the primary cluster.
          </p>
        )}
        {allowed.map((c) => (
          <div key={c.cluster_id} className="flex items-center gap-2 text-xs">
            <span className="font-mono">{c.cluster_id}</span>
            {c.region && <span className="text-muted-foreground/60">{c.region}</span>}
            {c.is_default ? (
              <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">default</span>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                className="h-5 px-1.5 text-[10px]"
                disabled={disabled || busy}
                onClick={() => makeDefault(c.cluster_id)}
                title="Route new deploys here when the user picks no cluster"
              >
                Make default
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon-xs"
              disabled={disabled || busy}
              onClick={() => remove(c.cluster_id)}
              title="Disallow this cluster. Fails while deployments still run on it."
            >
              <X className="size-3" />
            </Button>
          </div>
        ))}
      </div>

      <div className="mt-3 flex items-center gap-2">
        <Select value={toAdd} onValueChange={(v) => { setToAdd(v); setMessage(null); }} disabled={disabled || busy || addable.length === 0}>
          <SelectTrigger className="h-7 w-56 text-xs">
            <SelectValue placeholder={addable.length === 0 ? "No clusters left to add" : "Add a cluster…"} />
          </SelectTrigger>
          <SelectContent>
            {addable.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.id}
                {c.is_primary ? " — default cluster" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {toAdd && (
          <Button size="sm" className="h-7 px-2 text-xs" disabled={busy} onClick={add}>
            {addMut.isPending ? "Adding…" : "Add"}
          </Button>
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

function ObservabilityRow({ label, value, onRecover, pending, error, actionLabel = "Recover", href }: {
  label: string;
  value: string;
  onRecover: () => void;
  pending: boolean;
  error: Error | null;
  actionLabel?: string;
  href?: string;
}) {
  return (
    <div>
      <div className="flex items-center gap-2 text-xs">
        <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
        {value ? (
          <>
            <IdValue value={value} href={href} />
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
