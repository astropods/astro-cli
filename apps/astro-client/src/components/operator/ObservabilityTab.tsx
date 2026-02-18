import { useState, useMemo } from "react";
import { BarChart3, Loader2, ChevronLeft, ChevronRight } from "lucide-react";
import { useObservabilitySummary, useObservabilityTraces } from "../../api/queries/observability";

const PAGE_SIZE = 20;

export function ObservabilityTab({ account, agentName }: { account: string; agentName: string }) {
  const [offset, setOffset] = useState(0);

  const timeParams = useMemo(() => {
    const now = new Date();
    const start = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    return {
      start_time: start.toISOString(),
      end_time: now.toISOString(),
    };
  }, []);

  const traceParams = useMemo(
    () => ({ ...timeParams, limit: String(PAGE_SIZE), offset: String(offset) }),
    [timeParams, offset],
  );

  const { data: summary, isLoading: summaryLoading } = useObservabilitySummary(account, agentName, timeParams);
  const { data: traces, isLoading: tracesLoading } = useObservabilityTraces(account, agentName, traceParams);

  const isLoading = summaryLoading || tracesLoading;
  const isEmpty = !isLoading && !summary?.total_traces;

  if (isLoading && !summary && !traces) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={24} className="animate-spin text-stone-500" />
      </div>
    );
  }

  if (isEmpty) {
    return (
      <div className="p-8 border border-stone-300 bg-stone-50 text-center">
        <BarChart3 size={32} className="mx-auto text-stone-400 mb-2" />
        <p className="text-stone-600 text-sm">No observability data yet</p>
        <p className="text-stone-500 text-xs mt-1">
          Deploy your agent and send some requests to see traces here
        </p>
      </div>
    );
  }

  const metrics = summary?.metrics;
  const totalTraces = traces?.total ?? 0;
  const hasNext = offset + PAGE_SIZE < totalTraces;
  const hasPrev = offset > 0;

  const stats = [
    { label: "Total Traces", value: summary?.total_traces?.toLocaleString() ?? "—" },
    { label: "Avg Latency (ms)", value: metrics?.avg_latency_ms?.toFixed(1) ?? "—" },
    { label: "P95 Latency (ms)", value: metrics?.p95_latency_ms?.toFixed(1) ?? "—" },
    { label: "Total Tokens", value: metrics?.total_tokens?.toLocaleString() ?? "—" },
    { label: "Error Rate (%)", value: metrics?.error_rate != null ? (metrics.error_rate * 100).toFixed(1) : "—" },
  ];

  return (
    <div className="space-y-6">
      {/* Summary stat cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        {stats.map((s) => (
          <div key={s.label} className="border border-stone-300 bg-white p-4">
            <p className="text-xs text-stone-500 mb-1">{s.label}</p>
            <p className="text-xl font-semibold text-stone-900">{s.value}</p>
          </div>
        ))}
      </div>

      {/* Traces table */}
      {traces && traces.traces.length > 0 && (
        <div className="border border-stone-300 bg-white overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-stone-300 bg-stone-50 text-left text-stone-500">
                <th className="px-4 py-2.5 font-medium">Input</th>
                <th className="px-4 py-2.5 font-medium">Output</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium">Latency (ms)</th>
                <th className="px-4 py-2.5 font-medium">Timestamp</th>
              </tr>
            </thead>
            <tbody>
              {traces.traces.map((t) => (
                <tr key={t.trace_id} className="border-b border-stone-200 last:border-b-0">
                  <td className="px-4 py-2.5 text-xs max-w-[200px] truncate">{t.input}</td>
                  <td className="px-4 py-2.5 text-xs max-w-[300px] truncate">{t.output}</td>
                  <td className="px-4 py-2.5">
                    <StatusBadge status={t.status} />
                  </td>
                  <td className="px-4 py-2.5 tabular-nums">{t.latency_ms.toFixed(1)}</td>
                  <td className="px-4 py-2.5 text-stone-500">
                    {new Date(t.timestamp).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {/* Pagination */}
          {totalTraces > PAGE_SIZE && (
            <div className="flex items-center justify-between px-4 py-2.5 border-t border-stone-300 bg-stone-50 text-xs text-stone-500">
              <span>
                Showing {offset + 1}–{Math.min(offset + PAGE_SIZE, totalTraces)} of {totalTraces}
              </span>
              <div className="flex gap-1">
                <button
                  disabled={!hasPrev}
                  onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                  className="px-2 py-1 border border-stone-300 bg-white hover:bg-stone-50 disabled:opacity-40 cursor-pointer disabled:cursor-default"
                >
                  <ChevronLeft size={14} />
                </button>
                <button
                  disabled={!hasNext}
                  onClick={() => setOffset((o) => o + PAGE_SIZE)}
                  className="px-2 py-1 border border-stone-300 bg-white hover:bg-stone-50 disabled:opacity-40 cursor-pointer disabled:cursor-default"
                >
                  <ChevronRight size={14} />
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const lower = status.toLowerCase();
  const color =
    lower === "ok" || lower === "success"
      ? "text-green-700 bg-green-50 border-green-200"
      : lower === "error"
        ? "text-red-700 bg-red-50 border-red-200"
        : "text-yellow-700 bg-yellow-50 border-yellow-200";

  return (
    <span className={`inline-block px-1.5 py-0.5 text-xs border rounded ${color}`}>
      {status}
    </span>
  );
}
