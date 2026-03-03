import { useState } from "react";
import { useAgents, useAgentBuilds } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ChevronRight } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";

export function AgentsPage() {
  const { data, isLoading, error } = useAgents();
  const [selected, setSelected] = useState<{ account: string; name: string } | null>(null);

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Agents</h2>
      {isLoading && <LoadingSkeleton />}
      {error && <p className="text-red-400">Error: {error.message}</p>}
      {data && (
        <div className="flex gap-4">
          <div className="flex-1 overflow-x-auto rounded-md border border-stone-800">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-stone-800 bg-stone-900/50">
                  <th className="px-4 py-2 text-left font-medium text-stone-400">Account</th>
                  <th className="px-4 py-2 text-left font-medium text-stone-400">Name</th>
                  <th className="px-4 py-2 text-right font-medium text-stone-400">Builds</th>
                  <th className="px-4 py-2 text-right font-medium text-stone-400">Published</th>
                  <th className="px-4 py-2 text-left font-medium text-stone-400">Latest Build</th>
                  <th className="px-4 py-2 text-left font-medium text-stone-400">Updated</th>
                  <th className="w-8"></th>
                </tr>
              </thead>
              <tbody>
                {data.agents?.map((a) => {
                  const isSelected = selected?.account === a.account_name && selected?.name === a.name;
                  return (
                    <tr
                      key={`${a.account_name}/${a.name}`}
                      className={`cursor-pointer border-b border-stone-800/50 ${isSelected ? "bg-stone-800/50" : "hover:bg-stone-900/30"}`}
                      onClick={() => setSelected({ account: a.account_name, name: a.name })}
                    >
                      <td className="px-4 py-2 text-stone-400">{a.account_name}</td>
                      <td className="px-4 py-2 font-medium text-amber">{a.name}</td>
                      <td className="px-4 py-2 text-right">{a.build_count}</td>
                      <td className="px-4 py-2 text-right">{a.published_build_count}</td>
                      <td className="px-4 py-2 font-mono text-xs text-stone-500">{truncateUUID(a.latest_build_id)}</td>
                      <td className="px-4 py-2 text-stone-500">{formatDateTime(a.updated_at)}</td>
                      <td className="px-2">
                        <ChevronRight className="size-4 text-stone-600" />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {selected && (
            <BuildsPanel account={selected.account} name={selected.name} onClose={() => setSelected(null)} />
          )}
        </div>
      )}
    </div>
  );
}

function BuildsPanel({ account, name, onClose }: { account: string; name: string; onClose: () => void }) {
  const { data, isLoading, error } = useAgentBuilds(account, name);

  return (
    <div className="w-80 shrink-0 rounded-md border border-stone-800 bg-stone-900/30 p-3">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">
          {account}/{name} builds
        </h3>
        <Button variant="ghost" size="xs" onClick={onClose}>Close</Button>
      </div>
      {isLoading && <Skeleton className="h-32 w-full" />}
      {error && <p className="text-xs text-red-400">{error.message}</p>}
      {data?.builds?.map((b) => (
        <div key={b.build_id} className="mb-2 rounded border border-stone-800 px-2.5 py-1.5 text-xs">
          <p className="font-mono text-amber">{truncateUUID(b.build_id)}</p>
          {b.tagged_version && <p className="text-stone-400">v{b.tagged_version}</p>}
          <p className="text-stone-500">
            {b.published_at ? `Published ${formatDateTime(b.published_at)}` : `Updated ${formatDateTime(b.updated_at)}`}
          </p>
        </div>
      ))}
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
