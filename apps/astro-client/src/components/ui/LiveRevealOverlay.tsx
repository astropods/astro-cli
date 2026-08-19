import { useEffect, useMemo, useState } from "react";
import { ArrowRight, RefreshCw, Share2, X } from "lucide-react";
import type { AgentDeploymentSummary } from "@/lib/api";
import { formatDate } from "@/lib/deployment-utils";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { HoloCard } from "@/components/trading-card/HoloCard";
import { ShareBadgeDropdown } from "@/components/trading-card/ShareBadgeDropdown";
import { LiveRevealConfetti } from "@/components/ui/LiveRevealConfetti";
import { StatusBadge } from "@/components/StatusBadge";
import { useCardColors, useResolvedIntegrations } from "@/hooks/use-card-colors";
import { generateCard, NAME_MAX_CHARS, type CardData } from "astro-trading-card";
import { useDeploymentAvatarUrl } from "@/lib/avatar-bust";

function capDeploymentDisplayName(name: string): string {
  const trimmed = name.trim();
  const chars = Array.from(trimmed);
  if (chars.length <= NAME_MAX_CHARS) return trimmed;
  return chars.slice(0, NAME_MAX_CHARS).join("");
}

export function LiveRevealOverlay({
  deployment,
  account,
  accessReady = true,
  accessDelayed = false,
  accessStalled = false,
  deploymentStatus,
  onRetryAccess,
  onViewDeployment,
  onDismiss,
}: {
  deployment: AgentDeploymentSummary;
  account: string;
  accessReady?: boolean;
  accessDelayed?: boolean;
  accessStalled?: boolean;
  deploymentStatus?: string;
  onRetryAccess?: () => void;
  onViewDeployment: () => void;
  onDismiss: () => void;
}) {
  const [entered, setEntered] = useState(false);
  const { data: blueprint } = useBlueprint(account, deployment.name, { enabled: true });
  const integrations = blueprint ? getBlueprintIntegrations(blueprint) : [];

  useEffect(() => {
    let raf = 0;
    raf = window.requestAnimationFrame(() => {
      raf = window.requestAnimationFrame(() => setEntered(true));
    });
    return () => window.cancelAnimationFrame(raf);
  }, []);

  const avatarUrl = useDeploymentAvatarUrl(deployment);
  const deploymentDisplayName = capDeploymentDisplayName(deployment.display_name ?? deployment.name);
  const runtimeReady = ["active", "running"].includes(deploymentStatus?.toLowerCase() ?? "");
  const fullyReady = accessReady && runtimeReady;
  const stateLabel = accessStalled
    ? "Access setup needs attention"
    : !accessReady
      ? "Setting up access"
      : fullyReady
        ? "Active"
        : "Deploying";
  const stateTitle = accessStalled
    ? `${deploymentDisplayName} couldn’t finish access setup`
    : !accessReady
      ? `${deploymentDisplayName} is almost ready`
    : fullyReady
      ? `${deploymentDisplayName} is ready!`
      : `${deploymentDisplayName} is deploying!`;

  const baseCardData = useMemo<CardData>(() => {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return {
      name: deployment.name,
      displayName: deploymentDisplayName,
      account,
      avatar: { url: avatarUrl },
      stats: [
        { label: "Deployed", value: formatDate(deployment.created_at) },
        { label: "From", value: `${account}/${deployment.name}`, wrap: true },
      ],
      barcodeId: deployment.id,
      qrUrl: `${origin}/${account}/${deployment.name}`,
    };
  }, [account, avatarUrl, deployment.created_at, deployment.id, deployment.name, deploymentDisplayName]);

  const colors = useCardColors(deployment.avatar_colors);
  const cardIntegrations = useResolvedIntegrations(integrations, true);

  const revealCardData = useMemo<CardData>(
    () => ({
      ...baseCardData,
      colors,
      ...(cardIntegrations.length > 0 ? { integrations: cardIntegrations } : {}),
    }),
    [baseCardData, cardIntegrations, colors],
  );

  const revealCardSvg = useMemo(() => generateCard(revealCardData), [revealCardData]);
  const blueprintUrl = useMemo(() => {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return `${origin}/${account}/${deployment.name}`;
  }, [account, deployment.name]);

  return (
    <div
      className={cn(
        "fixed inset-0 z-40 flex items-center justify-center overflow-hidden p-6 transition-[background-color,backdrop-filter] duration-500 ease-out",
        entered
          ? "bg-black/70 backdrop-blur-md [transition-delay:120ms]"
          : "bg-transparent backdrop-blur-0 [transition-delay:0ms]",
      )}
      onMouseDown={onDismiss}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={onDismiss}
        className="absolute top-6 right-6 z-20 h-9 w-9 rounded-sm border border-white/35 bg-transparent text-white/80 shadow-none hover:border-white/55 hover:bg-transparent hover:text-white"
        aria-label="Close reveal"
      >
        <X className="size-4" />
      </Button>
      <LiveRevealConfetti />
      <div
        className="pointer-events-none absolute z-[1] h-[700px] w-[600px] rounded-full bg-[radial-gradient(ellipse,_rgba(21,130,125,0.18)_0%,_rgba(7,61,60,0.06)_50%,_transparent_70%)]"
      />

      <div
        className={cn(
          "relative z-10 flex w-fit max-w-[980px] flex-col items-center text-center transition-all duration-700 ease-out",
          entered ? "translate-y-0 opacity-100" : "translate-y-[18px] opacity-0",
        )}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex flex-col items-center gap-2">
          <StatusBadge
            color={accessStalled ? "error" : fullyReady ? "success" : "warning"}
            indicator
            spinning={!fullyReady && !accessStalled}
          >
            {accessDelayed && !accessReady && !accessStalled ? "Still setting up access" : stateLabel}
          </StatusBadge>
          <h1 className="mb-0 max-w-[min(92vw,760px)] text-[clamp(2rem,4vw,46px)] leading-[1.04] font-semibold tracking-tight text-white text-balance break-words drop-shadow-[0_2px_14px_rgba(0,0,0,0.45)] [overflow-wrap:anywhere]">
            {stateTitle}
          </h1>
          {!accessReady && (
            <p className="mt-2 max-w-[480px] text-sm text-white/70 dark:text-white/70">
              {accessStalled
                ? "We couldn’t confirm secure access. Retry the setup, or return to your agents and try again later."
                : accessDelayed
                  ? "Secure access is taking longer than usual. You can close this window and keep working."
                  : "We’re securing this deployment before it can be opened."}
            </p>
          )}
        </div>

        <div className="mt-6 flex w-[min(82vw,330px,33vh)] flex-col items-center gap-0">
          {/* Hidden preload keeps the avatar URL in the browser cache so the SVG
              <image> element doesn't flicker when the card is regenerated after
              color extraction completes. */}
          <img src={avatarUrl} aria-hidden className="sr-only" alt="" />
          <div
            className={cn(
              "w-full drop-shadow-[0_20px_50px_rgba(0,0,0,0.55)] transition-all duration-700 ease-out [&_.holo-card]:block [&_.holo-card]:w-full [&_.holo-card_svg]:h-auto [&_.holo-card_svg]:w-full",
              entered ? "translate-y-0 scale-[1.02] opacity-100" : "translate-y-4 scale-[0.98] opacity-0",
            )}
          >
            <HoloCard>
              <div
                style={{ borderRadius: 16, overflow: "hidden" }}
                dangerouslySetInnerHTML={{ __html: revealCardSvg }}
              />
            </HoloCard>
          </div>
        </div>

        <div className="mt-6 flex w-[min(82vw,330px,33vh)] flex-col items-stretch gap-2">
          {accessStalled ? (
            <>
              <Button
                variant="default"
                onClick={onRetryAccess}
                disabled={!onRetryAccess}
                className="w-full gap-2"
              >
                Retry access setup <RefreshCw className="size-4" />
              </Button>
              <Button
                variant="outline"
                onClick={onDismiss}
                className="w-full border-white/35 bg-transparent text-white shadow-none hover:border-white/55 hover:bg-white/10 hover:text-white dark:border-white/35 dark:bg-transparent dark:hover:border-white/55 dark:hover:bg-white/10"
              >
                Back to agents
              </Button>
            </>
          ) : !accessReady ? (
            <>
              <div
                role="status"
                aria-label="Deployment access is being configured"
                className="flex min-h-10 items-center justify-center gap-2 rounded-md border border-white/25 bg-white/5 px-4 text-sm font-medium text-white/85 dark:border-white/25 dark:bg-white/5 dark:text-white/85"
              >
                <RefreshCw className="size-4 motion-safe:animate-spin" />
                {accessDelayed ? "Still securing access" : "Securing deployment access"}
              </div>
              <Button
                variant="outline"
                onClick={onDismiss}
                className="w-full border-white/35 bg-transparent text-white shadow-none hover:border-white/55 hover:bg-white/10 hover:text-white dark:border-white/35 dark:bg-transparent dark:hover:border-white/55 dark:hover:bg-white/10"
              >
                Back to agents
              </Button>
            </>
          ) : (
            <Button
              variant="default"
              onClick={onViewDeployment}
              className="w-full gap-2"
            >
              View deployment <ArrowRight className="size-4" />
            </Button>
          )}
          {accessReady && !accessStalled && (
            <ShareBadgeDropdown
              launchName={deploymentDisplayName}
              blueprintUrl={blueprintUrl}
              svg={revealCardSvg}
              downloadName={deployment.name}
              downloadId={deployment.id}
            >
              <Button
                variant="outline"
                className="w-full gap-2 border-white/35 bg-transparent text-white shadow-none hover:border-white/55 hover:bg-white/10 hover:text-white dark:border-white/35 dark:bg-transparent dark:hover:border-white/55 dark:hover:bg-white/10"
              >
                <Share2 className="size-4" /> Share badge
              </Button>
            </ShareBadgeDropdown>
          )}
        </div>
      </div>
    </div>
  );
}
