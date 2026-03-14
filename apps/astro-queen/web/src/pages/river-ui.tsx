import { useRiverUIStatus, useStartRiverUI, useStopRiverUI } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { ExternalLink } from "lucide-react";

export function RiverUIPage() {
  const { data, isLoading } = useRiverUIStatus();
  const start = useStartRiverUI();
  const stop = useStopRiverUI();

  const running = data?.running ?? false;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">River UI</h2>
      <p className="text-sm text-muted-foreground">
        River UI provides a web dashboard for inspecting background job queues.
        It runs inside astro-server and is proxied through the admin gRPC boundary.
      </p>

      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <div
            className={`size-2 rounded-full ${running ? "bg-green-500" : "bg-muted-foreground/40"}`}
          />
          <span className="text-sm font-medium">
            {isLoading ? "Checking..." : running ? "Running" : "Stopped"}
          </span>
        </div>

        {running ? (
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => stop.mutate()}
              disabled={stop.isPending}
            >
              {stop.isPending ? "Stopping..." : "Stop"}
            </Button>
            <a href="/riverui/" target="_blank" rel="noopener noreferrer">
              <Button size="sm" variant="outline">
                Open <ExternalLink className="ml-1 size-3" />
              </Button>
            </a>
          </>
        ) : (
          <Button
            size="sm"
            onClick={() => start.mutate()}
            disabled={start.isPending}
          >
            {start.isPending ? "Starting..." : "Start"}
          </Button>
        )}
      </div>

      {(start.error || stop.error) && (
        <p className="text-destructive text-sm">
          {(start.error || stop.error)?.message}
        </p>
      )}

      {running && (
        <iframe
          src="/riverui/"
          className="w-full rounded border border-glass-border-honey"
          style={{ height: "calc(100vh - 220px)" }}
          title="River UI"
        />
      )}
    </div>
  );
}
