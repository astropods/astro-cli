import { Link } from "react-router";
import { CheckIcon, ClockIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Tag } from "@/components/Tag";
import { StatusBadge } from "@/components/StatusBadge";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";

export function PendingAcceptanceStage({
  store,
  account,
}: {
  store: KnowledgeStore;
  account: string;
}) {
  return (
    <div className="mx-auto max-w-lg flex flex-col items-center">

      <div className="flex flex-col items-center text-center mb-9 gap-1.5">
        <h2 className="text-heading-1 text-foreground">Complete your PrivateLink setup</h2>
        <p className="text-body text-muted-foreground max-w-sm">
          Your store is registered. Follow these steps to finish connecting it.
        </p>
      </div>

      {/* Stepper */}
      <div className="flex items-center mb-8">
        <div className="flex flex-col items-center gap-2">
          <div className="flex size-7 items-center justify-center rounded-full bg-success dark:bg-green-600 shrink-0">
            <CheckIcon className="size-3.5 text-white stroke-[2]" />
          </div>
          <span className="text-body-sm text-foreground w-max">Registered</span>
        </div>

        <div className="w-14 h-px mb-5 shrink-0 bg-success dark:bg-green-600" />

        <div className="flex flex-col items-center gap-2">
          <div
            className="flex size-7 items-center justify-center rounded-full border shrink-0"
            style={{
              background: "color-mix(in oklch, var(--color-yellow-500) 12%, transparent)",
              borderColor: "color-mix(in oklch, var(--color-yellow-500) 28%, transparent)",
            }}
          >
            <ClockIcon className="size-3.5 text-yellow-600 dark:text-yellow-400 stroke-[1.75]" />
          </div>
          <span className="text-body-sm text-foreground w-max">Awaiting approval</span>
        </div>

        <div className="w-14 h-px mb-5 shrink-0 bg-border" />

        <div className="flex flex-col items-center gap-2">
          <div className="size-7 rounded-full border-[1.5px] border-border shrink-0" />
          <span className="text-body-sm text-faint-foreground w-max">Connected</span>
        </div>
      </div>

      {/* Store card */}
      <div className="w-full rounded-lg overflow-hidden border border-border bg-white dark:bg-surface mb-4">
        <div className="flex items-center gap-3 px-4 py-4">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
            <ProviderIcon provider={store.provider} className="size-5" />
          </div>
          <div className="flex-1 min-w-0">
            <span className="font-medium text-foreground">{store.name}</span>
            <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[store.provider]}</p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Tag color={store.mode === "managed" ? "blue" : "default"}>
              {store.mode === "managed" ? "Managed" : "External"}
            </Tag>
            <StatusBadge color="warning" indicator spinning>Pending</StatusBadge>
          </div>
        </div>
      </div>

      <div className="w-full">
        <PrivateLinkSection store={store} showBanner={false} />
      </div>

      <div className="mt-8 w-full flex flex-col-reverse gap-2">
        <Button variant="ghost" asChild>
          <Link to={knowledgePath}>Back to stores</Link>
        </Button>
        <Button asChild>
          <Link to={knowledgeDetailPath(store.name, account)}>View store →</Link>
        </Button>
      </div>
    </div>
  );
}
