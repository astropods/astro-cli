import { useEffect } from "react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { PendingAcceptanceStage } from "./PendingAcceptanceStage";
import { Tag } from "@/components/Tag";
import { StatusBadge } from "@/components/StatusBadge";
import { Spinner } from "@/components/ui/spinner";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { useKnowledgeStore } from "@/api/queries/knowledge";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeProvider, KnowledgeStore } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

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

  const MANAGED_STEPS = [
    "Assigning resources",
    "Downloading database engine",
    "Database engine ready",
    "Preparing store",
    "Store starting up",
  ];
  const EXTERNAL_STEPS = [
    "Validating credentials",
    "Testing connection",
    "Saving configuration",
    "Verifying access",
    "Finalizing",
  ];
  const steps = mode === "managed" ? MANAGED_STEPS : EXTERNAL_STEPS;

  useEffect(() => {
    if (!store) return;
    if (store.status === "ready") {
      onReady(store);
    } else if (store.status === "error") {
      onError(store.error ?? "Provisioning failed");
    }
  }, [store, onReady, onError]);

  if (store?.status === "pending-acceptance") {
    return <PendingAcceptanceStage store={store} />;
  }

  const completedCount = Math.min(store?.events?.length ?? 0, steps.length);
  const heading = mode === "managed" ? "Setting up your store" : "Connecting your store";
  const subtitle = mode === "managed"
    ? "This may take a few moments."
    : "Verifying connectivity and saving credentials.";

  return (
    <div className="mx-auto max-w-lg">
      <div className="mb-8 text-center">
        <h2 className="text-heading-1 text-foreground">{heading}</h2>
        <p className="mt-1 text-body text-muted-foreground">{subtitle}</p>
      </div>

      <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface">
        <div className="flex items-center gap-3 px-4 py-4">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
            <ProviderIcon provider={provider} className="size-5" />
          </div>
          <div className="flex-1 min-w-0">
            <span className="font-medium text-foreground">{storeName}</span>
            <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[provider]}</p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Tag color={mode === "managed" ? "blue" : "default"}>{mode === "managed" ? "Managed" : "External"}</Tag>
            <StatusBadge color="warning" indicator spinning>Provisioning</StatusBadge>
          </div>
        </div>
      </div>

      <div className="mt-4 rounded-lg border border-border bg-white dark:bg-surface overflow-hidden">
        {steps.map((step, i) => {
          const isDone = i < completedCount;
          const isActive = i === completedCount;
          const isPending = i > completedCount;
          return (
            <div
              key={step}
              className={`flex items-center gap-3 px-4 py-3 transition-opacity duration-500 ${i < steps.length - 1 ? "border-b border-border/60" : ""} ${isPending ? "opacity-35" : "opacity-100"}`}
            >
              <div className="shrink-0">
                {isDone && (
                  <div className="flex size-5 items-center justify-center rounded-full bg-teal-600 dark:bg-teal-500">
                    <CheckIcon className="size-3 text-white stroke-[2.5]" />
                  </div>
                )}
                {isActive && (
                  <Spinner size={20} className="text-teal-600 dark:text-teal-500" />
                )}
                {isPending && (
                  <div className="size-5 rounded-full border-[1.5px] border-border" />
                )}
              </div>
              <span className={`text-body-sm ${isDone || isActive ? "text-foreground" : "text-muted-foreground"} ${isActive ? "font-medium" : ""}`}>
                {step}
              </span>
            </div>
          );
        })}
      </div>

      {store?.endpoint && (
        <div className="mt-4">
          <PrivateLinkSection store={store} />
        </div>
      )}
    </div>
  );
}
