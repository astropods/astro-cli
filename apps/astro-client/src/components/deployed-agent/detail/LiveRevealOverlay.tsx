import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Download, Share2 } from "lucide-react";
import type { AgentDeployment } from "@/lib/api";
import { formatDate } from "@/lib/deployment-utils";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
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
  onViewDeployment,
  onDismiss,
}: {
  deployment: AgentDeployment;
  account: string;
  onViewDeployment: () => void;
  onDismiss: () => void;
}) {
  const [entered, setEntered] = useState(false);
  const { data: blueprint } = useBlueprint(account, deployment.name, { enabled: true });
  const integrations = blueprint ? getBlueprintIntegrations(blueprint) : [];

  useEffect(() => {
    const timer = window.setTimeout(() => setEntered(true), 20);
    return () => window.clearTimeout(timer);
  }, []);

  const cardAvatar = useMemo<CardAvatar | undefined>(() => {
    const svg = generateIdentity({ seed: `${account}/${deployment.name}`, size: 128 });
    return { svg: stripSvgWrapper(svg) };
  }, [account, deployment.name]);

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
    return `${origin}/share/${account}/${deployment.name}`;
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
      className="fixed inset-0 z-40 flex items-center justify-center overflow-hidden p-6 transition-[background-color,backdrop-filter] duration-500 ease-out"
      onMouseDown={onDismiss}
      style={{
        backgroundColor: entered ? "rgba(0, 0, 0, 0.62)" : "rgba(0, 0, 0, 0)",
        backdropFilter: entered ? "blur(3px)" : "blur(0px)",
        WebkitBackdropFilter: entered ? "blur(3px)" : "blur(0px)",
        transitionDelay: entered ? "120ms" : "0ms",
      }}
    >
      <LiveRevealConfetti />
      <div
        className="pointer-events-none absolute z-[1] h-[700px] w-[600px] rounded-full"
        style={{
          background:
            "radial-gradient(ellipse, rgba(21,130,125,0.18) 0%, rgba(7,61,60,0.06) 50%, transparent 70%)",
        }}
      />

      <div
        className="relative z-10 flex w-fit max-w-[980px] flex-col items-center text-center transition-all duration-700 ease-out"
        onMouseDown={(event) => event.stopPropagation()}
        style={{
          opacity: entered ? 1 : 0,
          transform: entered ? "translateY(0)" : "translateY(18px)",
        }}
      >
        <div className="-mt-16 flex flex-col items-center gap-2">
          <span
            className="inline-flex w-fit items-center gap-2 rounded-full border px-3 py-1.5 font-mono text-label tracking-[0.08em]"
            style={{
              color: "var(--color-yellow-500)",
              borderColor: "color-mix(in oklch, var(--color-yellow-500) 28%, transparent)",
              backgroundColor: "color-mix(in oklch, var(--color-yellow-500) 12%, transparent)",
            }}
          >
            <span className="size-1.5 rounded-full" style={{ backgroundColor: "var(--color-yellow-500)" }} />
            DEPLOYING
          </span>
          <h1 className="mb-0 text-[46px] leading-[1.04] font-semibold tracking-tight text-white drop-shadow-[0_2px_14px_rgba(0,0,0,0.45)]">
            {(deployment.display_name ?? deployment.name)} is deploying!
          </h1>
        </div>

        <div className="mt-12 flex w-[min(82vw,330px)] flex-col items-center gap-0">
          <div
            className="w-full scale-[1.02] drop-shadow-[0_20px_50px_rgba(0,0,0,0.55)] transition-all duration-700 ease-out"
            style={{
              opacity: entered ? 1 : 0,
              transform: entered ? "translateY(0) scale(1.02)" : "translateY(16px) scale(0.98)",
            }}
          >
            <HoloCard>
              <div
                className="[&>svg]:h-auto [&>svg]:w-full"
                style={{ borderRadius: 16, overflow: "hidden", width: "100%" }}
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
