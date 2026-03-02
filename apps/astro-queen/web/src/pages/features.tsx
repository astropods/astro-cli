import { useState } from "react";
import { useFeatures, useCreateFeature, useDeleteFeature } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Plus, Trash2, X } from "lucide-react";
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

      {showCreate && (
        <CreateFeatureForm
          onClose={() => setShowCreate(false)}
          onSubmit={(body) => createMut.mutate(body, { onSuccess: () => setShowCreate(false) })}
          isPending={createMut.isPending}
        />
      )}

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-red-400 text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-md border border-zinc-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-zinc-800 bg-zinc-900/50">
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Key</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Name</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Meter Slug</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Archived</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Created</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((f) => (
                <tr key={f.id || f.key} className="border-b border-zinc-800/50 hover:bg-zinc-900/30">
                  <td className="px-4 py-2 font-mono text-xs text-amber">{f.key}</td>
                  <td className="px-4 py-2">{f.name}</td>
                  <td className="px-4 py-2 text-zinc-400">{f.meterSlug || "-"}</td>
                  <td className="px-4 py-2 text-zinc-500">{f.archivedAt ? "Yes" : "No"}</td>
                  <td className="px-4 py-2 text-zinc-500">{f.createdAt ? formatDateTime(f.createdAt) : "-"}</td>
                  <td className="px-4 py-2">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => { if (confirm(`Delete feature "${f.key}"?`)) deleteMut.mutate(f.id || f.key); }}
                    >
                      <Trash2 className="size-3 text-red-400" />
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

function CreateFeatureForm({ onClose, onSubmit, isPending }: { onClose: () => void; onSubmit: (body: Partial<Feature>) => void; isPending: boolean }) {
  const [form, setForm] = useState({ key: "", name: "", meterSlug: "" });
  const set = (k: string, v: string) => setForm((f) => ({ ...f, [k]: v }));

  return (
    <div className="rounded-md border border-zinc-800 bg-zinc-900/50 p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">Create Feature</h3>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3.5" /></Button>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Key *</label>
          <Input value={form.key} onChange={(e) => set("key", e.target.value)} />
        </div>
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Name</label>
          <Input value={form.name} onChange={(e) => set("name", e.target.value)} />
        </div>
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Meter Slug</label>
          <Input value={form.meterSlug} onChange={(e) => set("meterSlug", e.target.value)} />
        </div>
      </div>
      <div className="mt-3">
        <Button size="sm" onClick={() => onSubmit(form)} disabled={isPending || !form.key}>
          Create
        </Button>
      </div>
    </div>
  );
}
