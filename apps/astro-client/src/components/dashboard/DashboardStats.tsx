import { useMemo } from "react";
import { UsageCard } from "./ComputeUsageCard";
import { MetricCard } from "@/components/MetricCard";
import { useAccountObservabilitySummary } from "@/api/queries/observability";
import { useAccountUsage } from "@/api/queries/usage";
import { percentChange } from "@/components/deployed-agent/detail/monitor/trend-utils";

interface DashboardStatsProps {
  account: string;
  isLoading: boolean;
}

export function DashboardStats({
  account,
  isLoading,
}: DashboardStatsProps) {
  const { todayParams, yesterdayParams } = useMemo(() => {
    const now = new Date();
    const dayMs = 24 * 60 * 60 * 1000;
    return {
      todayParams: {
        start_time: new Date(now.getTime() - dayMs).toISOString(),
        end_time: now.toISOString(),
      },
      yesterdayParams: {
        start_time: new Date(now.getTime() - 2 * dayMs).toISOString(),
        end_time: new Date(now.getTime() - dayMs).toISOString(),
      },
    };
  }, []);

  const { data: todaySummary, isLoading: todayLoading } =
    useAccountObservabilitySummary(account, todayParams, { window: "24h" });
  const { data: yesterdaySummary, isLoading: yesterdayLoading } =
    useAccountObservabilitySummary(account, yesterdayParams, { window: "prev-24h" });

  const { data: usageData, isLoading: usageLoading } = useAccountUsage(account);

  const requestsToday = todaySummary?.total_traces ?? 0;
  const requestsTrend = useMemo(() => {
    const today = todaySummary?.total_traces;
    const yesterday = yesterdaySummary?.total_traces;
    if (today === 0) return null;
    return percentChange(today, yesterday);
  }, [todaySummary?.total_traces, yesterdaySummary?.total_traces]);

  const tokensToday = ((todaySummary?.input_tokens ?? 0) + (todaySummary?.output_tokens ?? 0));
  const tokensTrend = useMemo(() => {
    const today = todaySummary ? todaySummary.input_tokens + todaySummary.output_tokens : undefined;
    const yesterday = yesterdaySummary ? yesterdaySummary.input_tokens + yesterdaySummary.output_tokens : undefined;
    if (today === 0) return null;
    return percentChange(today, yesterday);
  }, [todaySummary, yesterdaySummary]);

  return (
    <div className="mb-9 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
      <MetricCard
        label="TOKENS TODAY"
        value={tokensToday.toLocaleString()}
        trend={tokensTrend}
        loading={isLoading || todayLoading}
        trendLoading={yesterdayLoading}
        className="bg-white dark:bg-background"
      />
      <MetricCard
        label="REQUESTS TODAY"
        value={requestsToday.toLocaleString()}
        trend={requestsTrend}
        loading={isLoading || todayLoading}
        trendLoading={yesterdayLoading}
        className="bg-white dark:bg-background"
      />
      <UsageCard
        label="COMPUTE USAGE"
        value={usageData?.compute_unit_hours.usage ?? 0}
        quota={usageData?.compute_unit_hours.quota}
        unit="CU-hours"
        link={{ href: "/settings/usage", label: "Request increase" }}
        loading={isLoading || usageLoading}
        className="bg-white dark:bg-background"
      />
    </div>
  );
}
