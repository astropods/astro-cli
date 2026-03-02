import { useState } from "react";
import { useQueryDatabase, useSchema } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Play, ChevronDown } from "lucide-react";

export function SqlQueryPage() {
  const [sql, setSql] = useState("");
  const queryMut = useQueryDatabase();
  const { data: schema, isLoading: schemaLoading } = useSchema();

  const groupedSchema = schema?.columns?.reduce(
    (acc, col) => {
      const table = col.table_name;
      if (!acc[table]) acc[table] = [];
      acc[table].push(col);
      return acc;
    },
    {} as Record<string, typeof schema.columns>
  );

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">SQL Query</h2>

      <div className="flex gap-4">
        <div className="flex-1 space-y-3">
          <div>
            <Textarea
              placeholder="SELECT * FROM accounts LIMIT 10"
              value={sql}
              onChange={(e) => setSql(e.target.value)}
              className="min-h-[120px] font-mono text-sm"
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                  e.preventDefault();
                  if (sql.trim()) queryMut.mutate(sql);
                }
              }}
            />
            <div className="mt-2 flex items-center gap-2">
              <Button
                size="sm"
                onClick={() => queryMut.mutate(sql)}
                disabled={!sql.trim() || queryMut.isPending}
              >
                <Play className="size-3.5" />
                Run Query
              </Button>
              <span className="text-xs text-zinc-500">Cmd+Enter</span>
            </div>
          </div>

          {queryMut.isPending && <Skeleton className="h-40 w-full" />}
          {queryMut.error && <p className="text-sm text-red-400">{queryMut.error.message}</p>}
          {queryMut.data && (
            <div className="overflow-x-auto rounded-md border border-zinc-800">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-zinc-800 bg-zinc-900/50">
                    {queryMut.data.columns?.map((col) => (
                      <th key={col} className="whitespace-nowrap px-3 py-1.5 text-left font-medium text-zinc-400">
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {queryMut.data.rows?.map((row, i) => (
                    <tr key={i} className="border-b border-zinc-800/50 hover:bg-zinc-900/30">
                      {row.values?.map((val, j) => (
                        <td key={j} className="max-w-xs truncate whitespace-nowrap px-3 py-1.5 font-mono text-zinc-300">
                          {val}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
              <p className="border-t border-zinc-800 px-3 py-1.5 text-xs text-zinc-500">
                {queryMut.data.rows?.length ?? 0} rows
              </p>
            </div>
          )}
        </div>

        <div className="w-64 shrink-0">
          <Collapsible defaultOpen>
            <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-zinc-400 hover:text-zinc-200">
              <ChevronDown className="size-4" />
              Schema
            </CollapsibleTrigger>
            <CollapsibleContent className="mt-2">
              {schemaLoading && <Skeleton className="h-32 w-full" />}
              {groupedSchema && (
                <div className="max-h-[60vh] space-y-2 overflow-y-auto">
                  {Object.entries(groupedSchema).map(([table, cols]) => (
                    <Collapsible key={table}>
                      <CollapsibleTrigger className="w-full text-left text-xs font-medium text-amber hover:text-amber/80">
                        {table}
                      </CollapsibleTrigger>
                      <CollapsibleContent className="ml-2 mt-1 space-y-0.5">
                        {cols.map((col) => (
                          <p key={col.column_name} className="text-xs">
                            <span className="text-zinc-300">{col.column_name}</span>{" "}
                            <span className="text-zinc-600">{col.data_type}</span>
                          </p>
                        ))}
                      </CollapsibleContent>
                    </Collapsible>
                  ))}
                </div>
              )}
            </CollapsibleContent>
          </Collapsible>
        </div>
      </div>
    </div>
  );
}
