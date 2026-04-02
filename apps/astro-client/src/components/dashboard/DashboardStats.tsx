import { UsageCard } from "./ComputeUsageCard";
import { MetricCard } from "@/components/MetricCard";
import { useAccountObservabilitySummary } from "@/api/queries/observability";
import { useAccountUsage } from "@/api/queries/usage";

interface DashboardStatsProps {
  account: string;
  isLoading: boolean;
}

export function DashboardStats({
  account,
  isLoading,
}: DashboardStatsProps) {
  const { data: allTimeSummary, isLoading: allTimeLoading } =
    useAccountObservabilitySummary(account, {}, { window: "all-time" });

  const { data: usageData, isLoading: usageLoading } = useAccountUsage(account);

  const totalRequests = allTimeSummary?.total_traces ?? 0;
  const totalTokens = (allTimeSummary?.input_tokens ?? 0) + (allTimeSummary?.output_tokens ?? 0);

  return (
    <div className="mb-9 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
      <MetricCard
        label="TOTAL TOKENS"
        value={totalTokens.toLocaleString()}
        loading={isLoading || allTimeLoading}
        className="bg-white dark:bg-background"
      />
      <MetricCard
        label="TOTAL REQUESTS"
        value={totalRequests.toLocaleString()}
        loading={isLoading || allTimeLoading}
        className="bg-white dark:bg-background"
      />
      <UsageCard
        label="TOTAL COMPUTE"
        value={usageData?.compute_unit_hours.usage ?? 0}
        quota={usageData?.compute_unit_hours.quota}
        unit="hours"
        account={account}
        loading={isLoading || usageLoading}
        className="bg-white dark:bg-background"
      />
    </div>
  );
}
