import { Link } from "react-router";
import type { WorkloadDetail } from "@/lib/api";

export function PodGrid({ workloads, basePath }: { workloads: WorkloadDetail[]; basePath: string }) {
  if (workloads.length === 0) {
    return <p className="text-sm text-muted-foreground">No workloads</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <h2 className="text-base font-semibold text-foreground">Services</h2>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {workloads.map((wl) => {
          const readyCount = wl.containers.filter((c) => c.ready).length;
          return (
            <Link
              key={wl.name}
              to={`${basePath}?workload=${encodeURIComponent(wl.name)}`}
              className="border border-border rounded-sm p-3 bg-background hover:bg-accent transition-colors"
            >
              <p className="text-sm font-medium truncate" title={wl.name}>{wl.component || wl.name}</p>
              <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                <span className="font-medium text-muted-foreground">{wl.kind}</span>
                <span>{readyCount}/{wl.containers.length} ready</span>
                <span>{wl.age}</span>
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
