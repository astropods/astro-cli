import { useFeatures, useCreateFeature, useDeleteFeature } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Archive, AlertTriangle } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { SchemaFormPanel } from "@/components/schema-form-panel";
import type { Feature } from "@/types/openmeter";

export function FeaturesPage() {
  const { data, isLoading, error } = useFeatures();
  const createMut = useCreateFeature();
  const deleteMut = useDeleteFeature();

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Features</h2>

      <div className="flex gap-4 items-start">
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-1 text-[10px] text-muted-foreground mb-1.5">
            <AlertTriangle className="size-2.5 text-amber shrink-0" />
            Features cannot be deleted, only archived.
          </p>
          {isLoading && <Skeleton className="h-40 w-full" />}
          {error && <p className="text-destructive text-sm">{error.message}</p>}
          {data && (
            <div className="overflow-x-auto rounded-lg glass">
              <table className="w-full text-[11px] whitespace-nowrap">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Key</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Meter Slug</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Archived</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Archive</th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((f) => (
                    <tr key={f.id || f.key} className="border-b border-comb-light hover:bg-glass-light">
                      <td className="px-2 py-0.5 font-mono text-xs text-amber">{f.key}</td>
                      <td className="px-2 py-0.5">{f.name}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{f.meterSlug || "-"}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{f.archivedAt ? "Yes" : "No"}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{f.createdAt ? formatDateTime(f.createdAt) : "-"}</td>
                      <td className="px-2 py-0.5">
                        <Button variant="ghost" size="icon-xs" title="Archive (features cannot be deleted)" onClick={() => { if (confirm(`Archive feature "${f.key}"? Features cannot be deleted, only archived.`)) deleteMut.mutate(f.id || f.key); }}><Archive className="size-3 text-amber" /></Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <SchemaFormPanel
          title="Create Feature"
          description="Define a new feature linked to a meter."
          schemaRef="FeatureCreate"
          onSubmit={(body) => createMut.mutate(body as Partial<Feature>)}
          isPending={createMut.isPending}
        />
      </div>
    </div>
  );
}
