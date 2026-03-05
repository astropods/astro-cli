import { useState } from "react";
import { useFeatures, useCreateFeature, useDeleteFeature } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Plus, Trash2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { Feature } from "@/types/openmeter";

export function FeaturesPage() {
  const { data, isLoading, error } = useFeatures();
  const createMut = useCreateFeature();
  const deleteMut = useDeleteFeature();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Features</h2>
        <Button size="sm" onClick={() => setShowCreate(true)}>
          <Plus className="size-3.5" /> Create Feature
        </Button>
      </div>

      <CreateFeatureDialog
        open={showCreate}
        onOpenChange={setShowCreate}
        onSubmit={(body) => createMut.mutate(body, { onSuccess: () => setShowCreate(false) })}
        isPending={createMut.isPending}
      />

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Key</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Meter Slug</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Archived</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((f) => (
                <tr key={f.id || f.key} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-4 py-2 font-mono text-xs text-amber">{f.key}</td>
                  <td className="px-4 py-2">{f.name}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.meterSlug || "-"}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.archivedAt ? "Yes" : "No"}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.createdAt ? formatDateTime(f.createdAt) : "-"}</td>
                  <td className="px-4 py-2">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => { if (confirm(`Delete feature "${f.key}"?`)) deleteMut.mutate(f.id || f.key); }}
                    >
                      <Trash2 className="size-3 text-red-500" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function CreateFeatureDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: Partial<Feature>) => void; isPending: boolean }) {
  const [form, setForm] = useState({ key: "", name: "", meterSlug: "" });
  const set = (k: string, v: string) => setForm((f) => ({ ...f, [k]: v }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Create Feature</DialogTitle>
          <DialogDescription>Define a new feature linked to a meter.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>Key <span className="text-red-500">*</span></Label>
            <Input value={form.key} onChange={(e) => set("key", e.target.value)} />
          </div>
          <div>
            <Label>Name</Label>
            <Input value={form.name} onChange={(e) => set("name", e.target.value)} />
          </div>
          <div>
            <Label>Meter Slug</Label>
            <Input value={form.meterSlug} onChange={(e) => set("meterSlug", e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button size="sm" onClick={() => onSubmit(form)} disabled={isPending || !form.key}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
