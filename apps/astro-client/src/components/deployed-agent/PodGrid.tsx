import { Link } from "react-router";
import type { PodDetail } from "@/lib/api";

function phaseColor(phase: string): string {
  switch (phase) {
    case "Running": return "text-green-600";
    case "Pending": return "text-yellow-600";
    case "Failed": return "text-red-600";
    case "Succeeded": return "text-blue-600";
    default: return "text-muted-foreground";
  }
}

export function PodGrid({ pods, basePath }: { pods: PodDetail[]; basePath: string }) {
  if (pods.length === 0) {
    return <p className="text-sm text-muted-foreground">No pods</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <h2 className="text-base font-semibold text-foreground">Containers</h2>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {pods.map((pod) => {
          const readyCount = pod.containers.filter((c) => c.ready).length;
          return (
            <Link
              key={pod.name}
              to={`${basePath}?pod=${encodeURIComponent(pod.name)}`}
              className="border border-border rounded-sm p-3 bg-card hover:bg-card-hover transition-colors"
            >
              <p className="font-mono text-sm truncate" title={pod.name}>{pod.name}</p>
              <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                <span className={`font-medium ${phaseColor(pod.phase)}`}>{pod.phase}</span>
                <span>{readyCount}/{pod.containers.length} ready</span>
                <span>{pod.age}</span>
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
