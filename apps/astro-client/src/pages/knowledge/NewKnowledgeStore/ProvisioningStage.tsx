import { useEffect } from "react";
import { ExclamationTriangleIcon, InformationCircleIcon } from "@heroicons/react/24/outline";
import { Spinner } from "@/components/ui/spinner";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { useKnowledgeStore } from "@/api/queries/knowledge";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeProvider, KnowledgeStore } from "@/lib/api";
import { ProviderIcon } from "./ProviderIcon";

export function ProvisioningStage({
  account,
  storeName,
  provider,
  mode,
  onReady,
  onError,
}: {
  account: string;
  storeName: string;
  provider: KnowledgeProvider;
  mode: "managed" | "external";
  onReady: (store: KnowledgeStore) => void;
  onError: (error: string) => void;
}) {
  const { data: store } = useKnowledgeStore(account, storeName);

  useEffect(() => {
    if (!store) return;
    if (store.status === "ready") {
      onReady(store);
    } else if (store.status === "error") {
      onError(store.error ?? "Provisioning failed");
    }
  }, [store, onReady, onError]);

  const events = store?.events ?? [];
  const heading = mode === "managed" ? "Provisioning your store" : "Connecting your store";
  const subtitle = mode === "managed"
    ? "Setting up infrastructure. This usually takes a moment."
    : "Verifying connectivity and saving credentials.";

  return (
    <div className="mx-auto max-w-lg">
      <div className="flex flex-col items-center text-center">
        <Spinner size={40} className="text-teal-600" />
        <h2 className="mt-6 text-heading-4 text-foreground">{heading}</h2>
        <p className="mt-1 text-body-sm text-muted-foreground">{subtitle}</p>
      </div>

      <div className="mt-8 rounded-lg border border-border bg-surface p-5">
        <div className="flex items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
            <ProviderIcon provider={provider} className="size-6" />
          </div>
          <div className="min-w-0">
            <span className="font-medium text-foreground">{storeName}</span>
            <p className="text-body-sm text-muted-foreground">
              {PROVIDER_LABELS[provider]} &middot; {mode === "managed" ? "Managed" : "External"}
            </p>
          </div>
        </div>
      </div>

      {events.length > 0 && (
        <div className="mt-4 space-y-2">
          {events.map((event, i) => {
            const isWarning = event.type === "Warning";
            return (
              <div key={i} className="flex items-start gap-3 rounded-md border border-border bg-surface px-4 py-3">
                {isWarning ? (
                  <ExclamationTriangleIcon className="size-4 shrink-0 mt-0.5 text-yellow-600" />
                ) : (
                  <InformationCircleIcon className="size-4 shrink-0 mt-0.5 text-blue-600" />
                )}
                <div className="flex-1 min-w-0">
                  <span className="font-medium text-body-sm text-foreground">{event.reason}</span>
                  <span className="text-body-sm text-muted-foreground">: {event.message}</span>
                </div>
                {event.count > 1 && (
                  <span className="shrink-0 rounded-full border border-border px-1.5 py-0.5 font-mono text-mono-sm text-muted-foreground">
                    x{event.count}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      )}

      {store?.endpoint && (
        <div className="mt-4">
          <PrivateLinkSection store={store} />
        </div>
      )}
    </div>
  );
}
