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

  const totalRequests = allTimeSummary?.totals?.requests ?? 0;
  const totalTokens = (allTimeSummary?.totals?.input_tokens ?? 0) + (allTimeSummary?.totals?.output_tokens ?? 0);

  return (
    <div className="mb-9 grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[800px]:grid-cols-3 @[1100px]:grid-cols-4">
      <MetricCard
        label="TOTAL TOKENS"
        value={totalTokens.toLocaleString()}
        showTrend={false}
        loading={isLoading || allTimeLoading}
      />
      <MetricCard
        label="TOTAL REQUESTS"
        value={totalRequests.toLocaleString()}
        showTrend={false}
        loading={isLoading || allTimeLoading}
      />
      <UsageCard
        label="TOTAL COMPUTE"
        value={usageData?.meters?.compute?.usage ?? 0}
        quota={usageData?.meters?.compute?.quota}
        unit="hours"
        account={account}
        loading={isLoading || usageLoading}
      />
    </div>
  );
}
