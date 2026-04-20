import { CheckIcon, ExclamationTriangleIcon } from "@heroicons/react/24/outline";
import { Spinner } from "@/components/ui/spinner";
import type { KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";

const STEPS = [
  { key: "connecting", label: "Creating endpoint" },
  { key: "pending-acceptance", label: "Waiting for acceptance" },
  { key: "ready", label: "Ready" },
];

export function PrivateLinkSection({ store }: { store: KnowledgeStore }) {
  if (!store.endpoint) return null;
  const status = store.endpoint.status;

  const currentIdx = STEPS.findIndex((s) => s.key === status);
  const isError = status === "error";

  return (
    <div className="rounded-lg border border-border bg-surface p-5">
      <h3 className="text-heading-4 text-foreground mb-4">PrivateLink</h3>

      <div className="space-y-3">
        {STEPS.map((step, i) => {
          const isActive = step.key === status;
          const isDone = !isError && (currentIdx > i || (isActive && status === "ready"));
          return (
            <div key={step.key} className="flex items-center gap-3">
              <div className={cn(
                "flex size-6 items-center justify-center rounded-full text-xs font-medium",
                isDone && "bg-teal-100 text-teal-700",
                isActive && !isError && "bg-yellow-100 text-yellow-700",
                !isDone && !isActive && "bg-muted text-muted-foreground",
              )}>
                {isDone ? <CheckIcon className="size-3.5" /> : isActive && !isError ? <Spinner size={14} /> : i + 1}
              </div>
              <span className={cn("text-body-sm", (isDone || isActive) ? "text-foreground" : "text-muted-foreground")}>
                {step.label}
              </span>
            </div>
          );
        })}

        {status === "pending-acceptance" && (
          <div className="flex items-start gap-3 rounded-md border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800">
            <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5 text-yellow-600" />
            <div>
              <p className="font-medium">Action required</p>
              <p>Accept the endpoint connection request in your AWS console.</p>
              {store.endpoint?.region && (
                <p className="mt-1 text-xs text-yellow-700">
                  Region: {store.endpoint.region}
                  {store.endpoint.endpoint_id && <> &middot; Endpoint: {store.endpoint.endpoint_id}</>}
                </p>
              )}
            </div>
          </div>
        )}

        {isError && store.endpoint?.error && (
          <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
            <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5" />
            <div>{store.endpoint.error}</div>
          </div>
        )}
      </div>
    </div>
  );
}
