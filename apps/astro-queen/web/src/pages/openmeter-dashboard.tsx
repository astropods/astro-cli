import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { api } from "@/lib/api";
import { useCustomers, useMeters } from "@/api/openmeter";
import { openmeterKeys } from "@/api/keys";
import { Skeleton } from "@/components/ui/skeleton";
import type { Customer, MeterQueryResult } from "@/types/openmeter";

// The 5 features and their limits from the private_beta plan.
// These match the issueAfterReset values in the plan definition.
const FEATURES: { key: string; meterSlug: string; limit: number; unit: string }[] = [
  { key: "compute", meterSlug: "compute", limit: 100, unit: "CU-hrs" },
  { key: "agents", meterSlug: "agents", limit: 5, unit: "agents" },
  { key: "agent_builds", meterSlug: "agent_builds", limit: 50, unit: "builds" },
  { key: "agent_deployments", meterSlug: "agent_deployments", limit: 10, unit: "deploys" },
  { key: "members", meterSlug: "members", limit: 5, unit: "members" },
];

// Query a meter with groupBy=subject to get per-customer totals in one call.
function useMeterBySubject(slug: string, enabled: boolean) {
  return useQuery({
    queryKey: [...openmeterKeys.meterQuery(slug), "bySubject"],
    queryFn: () =>
      api.get<MeterQueryResult>(
        `/api/openmeter/api/v1/meters/${encodeURIComponent(slug)}/query?groupBy=subject`
      ),
    enabled,
    staleTime: 60_000,
  });
}

interface CustomerUsage {
  customer: Customer;
  usage: Record<string, number>; // featureKey → value
}

function buildUsageTable(
  customers: Customer[],
  meterResults: Record<string, MeterQueryResult | undefined>,
): CustomerUsage[] {
  // Build per-customer usage
  const rows: CustomerUsage[] = [];
  for (const customer of customers) {
    const usage: Record<string, number> = {};
    for (const feat of FEATURES) {
      const result = meterResults[feat.meterSlug];
      const row = result?.data?.find((r) => r.subject === customer.key);
      usage[feat.key] = row?.value ?? 0;
    }
    rows.push({ customer, usage });
  }

  // Sort by highest total usage percentage descending
  rows.sort((a, b) => {
    const pctA = FEATURES.reduce((s, f) => s + (a.usage[f.key] ?? 0) / f.limit, 0);
    const pctB = FEATURES.reduce((s, f) => s + (b.usage[f.key] ?? 0) / f.limit, 0);
    return pctB - pctA;
  });

  return rows;
}

function pct(usage: number, limit: number): number {
  if (limit <= 0) return 0;
  return Math.min(100, (usage / limit) * 100);
}

function fmtVal(v: number): string {
  if (v === 0) return "0";
  if (v < 0.01) return v.toExponential(1);
  if (v < 10) return v.toFixed(2);
  if (v < 1000) return v.toFixed(1);
  return v.toLocaleString(undefined, { maximumFractionDigits: 0 });
}

function UsageBar({ value, limit }: { value: number; limit: number }) {
  const p = pct(value, limit);
  const color = p >= 100 ? "bg-red-500" : p >= 75 ? "bg-amber-500" : "bg-green-500";
  return (
    <div className="flex items-center gap-1.5 min-w-[120px]">
      <div className="h-1.5 flex-1 rounded-full bg-muted/30">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${Math.min(100, p)}%` }} />
      </div>
      <span className="text-[9px] text-muted-foreground w-8 text-right">{p.toFixed(0)}%</span>
    </div>
  );
}

export function OpenMeterDashboardPage() {
  const { data: customers, isLoading: loadingCustomers } = useCustomers();
  const { data: meters, isLoading: loadingMeters } = useMeters();

  const metersReady = !!meters && meters.length > 0;
  const computeQ = useMeterBySubject("compute", metersReady);
  const agentsQ = useMeterBySubject("agents", metersReady);
  const buildsQ = useMeterBySubject("agent_builds", metersReady);
  const deploymentsQ = useMeterBySubject("agent_deployments", metersReady);
  const membersQ = useMeterBySubject("members", metersReady);

  const meterResults: Record<string, MeterQueryResult | undefined> = {
    compute: computeQ.data,
    agents: agentsQ.data,
    agent_builds: buildsQ.data,
    agent_deployments: deploymentsQ.data,
    members: membersQ.data,
  };

  const isLoading = loadingCustomers || loadingMeters || computeQ.isLoading || agentsQ.isLoading || buildsQ.isLoading || deploymentsQ.isLoading || membersQ.isLoading;

  const rows = useMemo(
    () => (customers ? buildUsageTable(customers, meterResults) : []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [customers, computeQ.data, agentsQ.data, buildsQ.data, deploymentsQ.data, membersQ.data],
  );

  // Aggregate stats: per-feature totals + overall
  const aggregates = useMemo(() => {
    const total = customers?.length ?? 0;
    const withUsage = rows.filter((r) => FEATURES.some((f) => (r.usage[f.key] ?? 0) > 0)).length;
    const overLimit = rows.filter((r) => FEATURES.some((f) => (r.usage[f.key] ?? 0) >= f.limit)).length;

    const perFeature = FEATURES.map((f) => {
      const values = rows.map((r) => r.usage[f.key] ?? 0);
      const totalUsage = values.reduce((s, v) => s + v, 0);
      const activeCount = values.filter((v) => v > 0).length;
      const atLimit = values.filter((v) => v >= f.limit).length;
      const maxUsage = values.length > 0 ? Math.max(...values) : 0;
      const avgUsage = activeCount > 0 ? totalUsage / activeCount : 0;
      const sorted = [...values].sort((a, b) => a - b);
      const p95 = sorted.length > 0 ? sorted[Math.ceil(sorted.length * 0.95) - 1] : 0;
      const p99 = sorted.length > 0 ? sorted[Math.ceil(sorted.length * 0.99) - 1] : 0;
      return { ...f, totalUsage, activeCount, atLimit, maxUsage, avgUsage, p95, p99 };
    });

    return { total, withUsage, overLimit, perFeature };
  }, [customers, rows]);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Usage Dashboard</h2>

      {/* Aggregate cards */}
      <div className="grid grid-cols-3 gap-3">
        <div className="glass rounded-lg px-3 py-2">
          <div className="text-[10px] text-muted-foreground">Total Customers</div>
          <div className="text-lg font-semibold">{isLoading ? "-" : aggregates.total}</div>
        </div>
        <div className="glass rounded-lg px-3 py-2">
          <div className="text-[10px] text-muted-foreground">With Usage</div>
          <div className="text-lg font-semibold">{isLoading ? "-" : aggregates.withUsage}</div>
        </div>
        <div className="glass rounded-lg px-3 py-2">
          <div className="text-[10px] text-muted-foreground">At/Over Limit</div>
          <div className="text-lg font-semibold text-red-500">{isLoading ? "-" : aggregates.overLimit}</div>
        </div>
      </div>

      {/* Per-feature aggregates */}
      {!isLoading && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Feature</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Limit</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Active</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">At Limit</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Total Usage</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Avg (active)</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">P95</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">P99</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Max</th>
              </tr>
            </thead>
            <tbody>
              {aggregates.perFeature.map((f) => (
                <tr key={f.key} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5 font-medium capitalize">{f.key.replace(/_/g, " ")}</td>
                  <td className="px-2 py-0.5 text-right text-muted-foreground">{f.limit} {f.unit}</td>
                  <td className="px-2 py-0.5 text-right">{f.activeCount}</td>
                  <td className="px-2 py-0.5 text-right">
                    {f.atLimit > 0 ? <span className="text-red-500">{f.atLimit}</span> : "0"}
                  </td>
                  <td className="px-2 py-0.5 text-right">{fmtVal(f.totalUsage)} {f.unit}</td>
                  <td className="px-2 py-0.5 text-right">{fmtVal(f.avgUsage)} {f.unit}</td>
                  <td className="px-2 py-0.5 text-right">
                    <span className={pct(f.p95, f.limit) >= 100 ? "text-red-500" : pct(f.p95, f.limit) >= 75 ? "text-amber-500" : ""}>
                      {fmtVal(f.p95)} {f.unit}
                    </span>
                  </td>
                  <td className="px-2 py-0.5 text-right">
                    <span className={pct(f.p99, f.limit) >= 100 ? "text-red-500" : pct(f.p99, f.limit) >= 75 ? "text-amber-500" : ""}>
                      {fmtVal(f.p99)} {f.unit}
                    </span>
                  </td>
                  <td className="px-2 py-0.5 text-right">
                    <span className={pct(f.maxUsage, f.limit) >= 75 ? "text-amber-500" : pct(f.maxUsage, f.limit) >= 100 ? "text-red-500" : ""}>
                      {fmtVal(f.maxUsage)} {f.unit}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {isLoading && <Skeleton className="h-60 w-full" />}

      {!isLoading && (
        <>
          {/* Per-customer usage table */}
          <div className="overflow-x-auto rounded-lg glass">
            <table className="w-full text-[11px] whitespace-nowrap">
              <thead>
                <tr className="border-b border-glass-border-honey glass-subtle">
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Customer</th>
                  {FEATURES.map((f) => (
                    <th key={f.key} className="px-2 py-0.5 text-left font-medium text-muted-foreground capitalize">
                      {f.key.replace(/_/g, " ")}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.customer.id} className="border-b border-comb-light hover:bg-glass-light">
                    <td className="px-2 py-0.5">
                      <Link to={`/openmeter/customers/${r.customer.id}`} className="text-amber hover:underline">
                        {r.customer.name}
                      </Link>
                    </td>
                    {FEATURES.map((f) => (
                      <td key={f.key} className="px-2 py-0.5">
                        <div className="flex flex-col gap-0.5">
                          <span className="text-[10px]">
                            {fmtVal(r.usage[f.key] ?? 0)} / {f.limit} {f.unit}
                          </span>
                          <UsageBar value={r.usage[f.key] ?? 0} limit={f.limit} />
                        </div>
                      </td>
                    ))}
                  </tr>
                ))}
                {rows.length === 0 && (
                  <tr>
                    <td colSpan={FEATURES.length + 1} className="px-2 py-4 text-center text-muted-foreground">
                      No customers found.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
