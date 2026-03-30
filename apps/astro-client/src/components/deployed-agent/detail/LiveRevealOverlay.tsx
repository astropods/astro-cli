import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Download, Share2, X } from "lucide-react";
import type { AgentDeployment } from "@/lib/api";
import { formatDate } from "@/lib/deployment-utils";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { HoloCard } from "@/components/trading-card/HoloCard";
import { LiveRevealConfetti } from "@/components/deployed-agent/detail/LiveRevealConfetti";
import { useExtractedColors, useResolvedIntegrations } from "@/components/deployed-agent/detail/liveRevealCardHooks";
import { generateIdentity } from "identity-gen";
import type { CardAvatar, CardData } from "astro-trading-card";
import { generateCard, stripSvgWrapper } from "astro-trading-card";

export function LiveRevealOverlay({
  deployment,
  account,
  fallbackAvatarUrl,
  onViewDeployment,
  onDismiss,
}: {
  deployment: AgentDeployment;
  account: string;
  fallbackAvatarUrl?: string;
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

  const cardAvatar = useMemo<CardAvatar | undefined>(() => {
    const avatarUrl = deployment.avatar_url ?? fallbackAvatarUrl;
    if (avatarUrl) {
      return { url: avatarUrl };
    }
    const svg = generateIdentity({ seed: `${account}/${deployment.name}`, size: 128 });
    return { svg: stripSvgWrapper(svg) };
  }, [account, deployment.avatar_url, deployment.name, fallbackAvatarUrl]);

  const baseCardData = useMemo<CardData>(() => {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return {
      name: deployment.name,
      displayName: deployment.display_name,
      account,
      avatar: cardAvatar,
      stats: [
        { label: "Deployed", value: formatDate(deployment.created_at) },
        { label: "From", value: `${account}/${deployment.name}` },
      ],
      barcodeId: deployment.id,
      qrUrl: `${origin}/${account}/${deployment.name}`,
    };
  }, [account, cardAvatar, deployment.created_at, deployment.display_name, deployment.id, deployment.name]);

  const colors = useExtractedColors(baseCardData.avatar, true);
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

  const handleShareToNetwork = (network: "x" | "linkedin") => {
    const launchName = deployment.display_name ?? deployment.name;
    const shareText = `Just launched ${launchName} on Astro AI!\n\nCheck out the blueprint:\n\n${blueprintUrl}`;
    const url = network === "x"
      ? `https://x.com/intent/post?text=${encodeURIComponent(shareText)}`
      : `https://www.linkedin.com/feed/?shareActive=true&mini=true&text=${encodeURIComponent(shareText)}`;

    window.open(url, "_blank", "noopener,noreferrer");
  };

  const handleDownload = async (format: "svg" | "png") => {
    const mod = await import("astro-trading-card/browser");
    const opts = { name: deployment.name, id: deployment.id };
    if (format === "svg") {
      await mod.downloadSvg(revealCardSvg, opts);
    } else {
      await mod.downloadPng(revealCardSvg, opts);
    }
  };

  return (
    <div
      className={cn(
        "fixed inset-0 z-40 flex items-center justify-center overflow-hidden p-6 transition-[background-color,backdrop-filter] duration-500 ease-out",
        entered
          ? "bg-black/[0.62] backdrop-blur-[3px] [transition-delay:120ms]"
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
        <div className="-mt-16 flex flex-col items-center gap-2">
          <span className="inline-flex w-fit items-center gap-2 rounded-full border border-[color-mix(in_oklch,_var(--color-yellow-500)_28%,_transparent)] bg-[color-mix(in_oklch,_var(--color-yellow-500)_12%,_transparent)] px-3 py-1.5 font-mono text-label tracking-[0.08em] text-[var(--color-yellow-500)]">
            <span className="size-1.5 rounded-full bg-[var(--color-yellow-500)]" />
            DEPLOYING
          </span>
          <h1 className="mb-0 text-[46px] leading-[1.04] font-semibold tracking-tight text-white drop-shadow-[0_2px_14px_rgba(0,0,0,0.45)]">
            {(deployment.display_name ?? deployment.name)} is deploying!
          </h1>
        </div>

        <div className="mt-12 flex w-[min(82vw,330px)] flex-col items-center gap-0">
          {/* Hidden preload keeps the avatar URL in the browser cache so the SVG
              <image> element doesn't flicker when the card is regenerated after
              color extraction completes. */}
          {(deployment.avatar_url ?? fallbackAvatarUrl) && (
            <img src={deployment.avatar_url ?? fallbackAvatarUrl} aria-hidden className="sr-only" alt="" />
          )}
          <div
            className={cn(
              "w-full drop-shadow-[0_20px_50px_rgba(0,0,0,0.55)] transition-all duration-700 ease-out",
              entered ? "translate-y-0 scale-[1.02] opacity-100" : "translate-y-4 scale-[0.98] opacity-0",
            )}
          >
            <HoloCard>
              <div
                className="w-full overflow-hidden rounded-2xl [&>svg]:h-auto [&>svg]:w-full"
                dangerouslySetInnerHTML={{ __html: revealCardSvg }}
              />
            </HoloCard>
          </div>
        </div>

        <div className="mt-10 flex w-[min(82vw,330px)] flex-col items-stretch gap-2">
          <Button
            variant="default"
            onClick={onViewDeployment}
            className="w-full gap-2 border-0 bg-teal-700 text-white hover:bg-teal-600 dark:bg-teal-600 dark:hover:bg-teal-500"
          >
            View deployment <ArrowRight className="size-4" />
          </Button>
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" className="w-full gap-2 border-white/35 text-white hover:bg-white/10">
                <Share2 className="size-4" /> Share badge
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="center" sideOffset={6} className="w-fit min-w-0">
              <DropdownMenuItem onSelect={() => void handleShareToNetwork("x")} className="gap-2">
                <span className="inline-flex size-4 items-center justify-center rounded-[3px] border border-current text-[10px] font-semibold">
                  X
                </span>
                Share on X
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => void handleShareToNetwork("linkedin")} className="gap-2">
                <span className="inline-flex size-4 items-center justify-center rounded-[3px] border border-current text-[8px] font-bold leading-none">
                  in
                </span>
                Share on LinkedIn
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => void handleDownload("png")} className="gap-2">
                <Download className="size-4 text-current" />
                Download PNG
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => void handleDownload("svg")} className="gap-2">
                <Download className="size-4 text-current" />
                Download SVG
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );
}
