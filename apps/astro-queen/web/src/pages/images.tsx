import { useImages } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";

export function ImagesPage() {
  const { data, isLoading, error } = useImages();

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Images</h2>
      {isLoading && (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
        </div>
      )}
      {error && <p className="text-red-400">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-md border border-zinc-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-zinc-800 bg-zinc-900/50">
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Repository</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Namespace</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Name</th>
                <th className="px-4 py-2 text-left font-medium text-zinc-400">Tags</th>
              </tr>
            </thead>
            <tbody>
              {data.images?.map((img, i) => (
                <tr key={i} className="border-b border-zinc-800/50 hover:bg-zinc-900/30">
                  <td className="px-4 py-2 text-zinc-400">{img.repository}</td>
                  <td className="px-4 py-2 text-zinc-400">{img.namespace}</td>
                  <td className="px-4 py-2 font-medium">{img.name}</td>
                  <td className="px-4 py-2">
                    <div className="flex flex-wrap gap-1">
                      {img.tags?.map((tag) => (
                        <span key={tag} className="rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-xs text-zinc-300">
                          {tag}
                        </span>
                      ))}
                    </div>
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
