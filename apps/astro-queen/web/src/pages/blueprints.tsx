import { useState } from "react";
import { useBlueprints, useBlueprintBuilds } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ChevronRight } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";

export function BlueprintsPage() {
  const { data, isLoading, error } = useBlueprints();
  const [selected, setSelected] = useState<{ account: string; name: string } | null>(null);

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Blueprints</h2>
      {isLoading && <LoadingSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <div className="flex gap-4">
          <div className="flex-1 overflow-x-auto rounded-lg glass">
            <table className="w-full text-[11px] whitespace-nowrap">
              <thead>
                <tr className="border-b border-glass-border-honey glass-subtle">
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Account</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                  <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Builds</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Latest Build</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Updated</th>
                  <th className="w-8"></th>
                </tr>
              </thead>
              <tbody>
                {data.agents?.map((a) => {
                  const isSelected = selected?.account === a.account_name && selected?.name === a.name;
                  return (
                    <tr
                      key={`${a.account_name}/${a.name}`}
                      className={`cursor-pointer border-b border-comb-light ${isSelected ? "bg-pollen" : "hover:bg-glass-light"}`}
                      onClick={() => setSelected({ account: a.account_name, name: a.name })}
                    >
                      <td className="px-2 py-0.5 text-muted-foreground">{a.account_name}</td>
                      <td className="px-2 py-0.5 font-medium text-amber">{a.name}</td>
                      <td className="px-2 py-0.5 text-right">{a.build_count}</td>
                      <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{truncateUUID(a.latest_build_id)}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(a.updated_at)}</td>
                      <td className="px-2">
                        <ChevronRight className="size-4 text-muted-foreground" />
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
  const { data, isLoading, error } = useBlueprintBuilds(account, name);

  return (
    <div className="w-80 shrink-0 rounded-lg glass p-3">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">
          {account}/{name} builds
        </h3>
        <Button variant="ghost" size="xs" onClick={onClose}>Close</Button>
      </div>
      {isLoading && <Skeleton className="h-32 w-full" />}
      {error && <p className="text-xs text-destructive">{error.message}</p>}
      {data?.builds?.map((b) => (
        <div key={b.build_id} className="mb-2 rounded-lg border border-glass-border-honey px-2.5 py-1.5 text-xs">
          <p className="font-mono text-amber">{truncateUUID(b.build_id)}</p>
          <p className="text-muted-foreground">
            {formatDateTime(b.updated_at)}
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
