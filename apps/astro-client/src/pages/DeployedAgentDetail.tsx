import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useParams, Link } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { ArrowRight, Download, Share2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { HoloCard } from "@/components/trading-card/HoloCard";
import { useAgent } from "@/api/queries/agents";
import { useDeployments } from "@/api/queries/deployments";
import { resolveCardIntegrations } from "@/lib/integrationIcons";
import { getAgentIntegrations } from "@/lib/agent-utils";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { formatDate, isDeployingState, mapDeploymentStatus } from "@/lib/deployment-utils";
import { generateIdentity } from "identity-gen";
import type { CardAvatar, CardColors, CardData } from "astro-trading-card";
import { DEFAULT_COLORS, generateCard, stripSvgWrapper } from "astro-trading-card";
import { extractColorsFromImage, svgToImageSource } from "astro-trading-card/browser";
import type { AgentDeployment, ResolvedIntegration } from "@/lib/api";


export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  const deploymentsData = await api.listDeployments(account).catch(() => ({ deployments: [], count: 0 }));
  const deployment = deploymentsData.deployments?.find((d) => d.id === deploymentId) ?? null;

  return { deploymentsData, deployment, account, deploymentId };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const name = data?.deployment?.display_name || data?.deployment?.name || "Agent";
  return [{ title: `${name} | Astro` }];
};

function DeployedAgentDetailSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      <div className="flex items-center justify-between px-6 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
      <div className="mx-auto w-full max-w-3xl">
        <div className="flex items-center gap-4 px-6 py-6">
          <Skeleton className="size-14 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
        </div>
      </div>
    </div>
  );
}

function useExtractedColors(avatar: CardData["avatar"], enabled: boolean) {
  const [colors, setColors] = useState<CardColors>(DEFAULT_COLORS);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;

    let source: string | null = null;
    if (avatar?.url) {
      source = avatar.url;
    } else if (avatar?.svg) {
      source = svgToImageSource(avatar.svg);
    }

    if (!source) {
      setColors(DEFAULT_COLORS);
      return;
    }

    extractColorsFromImage(source).then((result) => {
      if (!cancelled) setColors(result ?? DEFAULT_COLORS);
    });

    return () => {
      cancelled = true;
    };
  }, [avatar, enabled]);

  return colors;
}

function useResolvedIntegrations(integrations: ResolvedIntegration[] | undefined, enabled: boolean) {
  return useMemo(
    () => (enabled && integrations?.length ? resolveCardIntegrations(integrations) : []),
    [enabled, integrations],
  );
}

function LiveRevealConfetti() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;

    const colors = ["#15827d", "#57c4c1", "#D48F1E", "#F0816A", "#073d3c", "#c4b89e", "#2d7a4f"];
    const pieces: {
      x: number;
      y: number;
      vx: number;
      vy: number;
      rot: number;
      vr: number;
      w: number;
      h: number;
      color: string;
      shape: "rect" | "circle";
    }[] = [];

    for (let i = 0; i < 120; i += 1) {
      pieces.push({
        x: Math.random() * canvas.width,
        y: -10 - Math.random() * 200,
        vx: (Math.random() - 0.5) * 3.1,
        vy: 2.1 + Math.random() * 3.6,
        rot: Math.random() * Math.PI * 2,
        vr: (Math.random() - 0.5) * 0.14,
        w: 6 + Math.random() * 8,
        h: 4 + Math.random() * 6,
        color: colors[Math.floor(Math.random() * colors.length)],
        shape: Math.random() > 0.5 ? "rect" : "circle",
      });
    }

    let raf = 0;
    const draw = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      let alive = false;

      for (const piece of pieces) {
        piece.x += piece.vx;
        piece.y += piece.vy;
        piece.rot += piece.vr;
        piece.vy += 0.045;
        if (piece.y < canvas.height + 20) alive = true;

        ctx.save();
        ctx.translate(piece.x, piece.y);
        ctx.rotate(piece.rot);
        ctx.fillStyle = piece.color;
        ctx.globalAlpha = Math.max(0, 1 - piece.y / canvas.height);
        if (piece.shape === "circle") {
          ctx.beginPath();
          ctx.arc(0, 0, piece.w / 2, 0, Math.PI * 2);
          ctx.fill();
        } else {
          ctx.fillRect(-piece.w / 2, -piece.h / 2, piece.w, piece.h);
        }
        ctx.restore();
      }

      if (alive) raf = window.requestAnimationFrame(draw);
    };

    const timer = window.setTimeout(() => {
      raf = window.requestAnimationFrame(draw);
    }, 180);

    return () => {
      window.clearTimeout(timer);
      window.cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none absolute inset-0 z-0"
    />
  );
}

function LiveRevealOverlay({
  deployment,
  account,
  onViewMonitoring,
}: {
  deployment: AgentDeployment;
  account: string;
  onViewMonitoring: () => void;
}) {
  const [entered, setEntered] = useState(false);
  const { data: agent } = useAgent(account, deployment.name, { enabled: true });
  const integrations = agent ? getAgentIntegrations(agent) : [];

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
      className="fixed inset-0 z-40 flex items-center justify-center overflow-hidden p-6 transition-[background-color,backdrop-filter] duration-500 ease-out"
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
        className="relative z-10 flex w-full max-w-[980px] flex-col items-center text-center transition-all duration-700 ease-out"
        style={{
          opacity: entered ? 1 : 0,
          transform: entered ? "translateY(0)" : "translateY(18px)",
        }}
      >
        <div className="-mt-10 flex flex-col items-center gap-2">
          <span className="inline-flex w-fit items-center gap-2 rounded-full border border-teal-300/45 bg-teal-400/18 px-3 py-1.5 font-mono text-label tracking-[0.08em] text-teal-100">
            <span className="size-1.5 rounded-full bg-teal-100" />
            LIVE
          </span>
          <h1 className="mb-0 text-[46px] leading-[1.04] font-semibold tracking-tight text-white drop-shadow-[0_2px_14px_rgba(0,0,0,0.45)]">
            {(deployment.display_name ?? deployment.name)} is live.
          </h1>
          <p className="mt-0 text-body text-stone-200/95">Monitoring begins on first request.</p>
        </div>

        <div className="mt-10 flex w-[min(82vw,330px)] flex-col items-center gap-0">
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
            onClick={onViewMonitoring}
            className="w-full gap-2 border-0 bg-teal-700 text-white hover:bg-teal-600 dark:bg-teal-600 dark:hover:bg-teal-500"
          >
            View monitoring <ArrowRight className="size-4" />
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
                <Download className="size-4" />
                Download PNG
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => void handleDownload("svg")} className="gap-2">
                <Download className="size-4" />
                Download SVG
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );
}

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const location = useLocation();
  const navigate = useNavigate();
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated, personalAccount } = useAuth();
  const [showLiveReveal, setShowLiveReveal] = useState(false);
  const [hasSeenReveal, setHasSeenReveal] = useState(false);
  const [hasLoadedRevealSeen, setHasLoadedRevealSeen] = useState(false);
  const [stayOnDeployments, setStayOnDeployments] = useState(false);
  const [allowMonitorTab, setAllowMonitorTab] = useState(false);
  const trackedDeploymentIdRef = useRef<string | null>(null);

  const { data: deploymentsData } = useDeployments(account, isAuthenticated);
  const deployments = deploymentsData?.deployments ?? loaderData?.deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId) ?? loaderData?.deployment ?? null;
  const currentDeploymentId = deployment?.id ?? null;
  const status = deployment ? mapDeploymentStatus(deployment) : null;
  const monitorLocked = deployment ? isDeployingState(deployment) : false;
  const isPersonal = personalAccount?.name === account;
  const queryTab = new URLSearchParams(location.search).get("tab");
  const queryFrom = new URLSearchParams(location.search).get("from");
  const requestedTab = queryTab === "monitor" || queryTab === "deployments" ? queryTab : null;
  const requestedFromAgents = queryFrom === "agents";
  const initialTab: "monitor" | "deployments" =
    (monitorLocked || stayOnDeployments)
      ? "deployments"
      : (requestedTab === "monitor" && (allowMonitorTab || requestedFromAgents) ? "monitor" : "deployments");
  const revealSeenKey = deployment
    ? `astro:deploy-live-reveal:${account}:${deployment.name}:${deployment.id}`
    : "";

  useEffect(() => {
    if (!currentDeploymentId) return;
    if (trackedDeploymentIdRef.current !== currentDeploymentId) {
      setAllowMonitorTab(false);
    }
    trackedDeploymentIdRef.current = currentDeploymentId;
    setShowLiveReveal(false);
    setStayOnDeployments(false);
    setHasLoadedRevealSeen(false);

    const revealSeen = typeof window !== "undefined" && window.localStorage.getItem(revealSeenKey) === "1";
    setHasSeenReveal(revealSeen);
    setHasLoadedRevealSeen(true);
  }, [currentDeploymentId, revealSeenKey]);

  useEffect(() => {
    if (!deployment || !status) return;
    if (!hasLoadedRevealSeen) return;
    if (trackedDeploymentIdRef.current !== deployment.id) return;
    if (status === "pending") {
      setStayOnDeployments(true);
      setShowLiveReveal(false);
      return;
    }
    if (status === "active" && !hasSeenReveal) {
      setShowLiveReveal(true);
      setHasSeenReveal(true);
      if (typeof window !== "undefined") {
        window.localStorage.setItem(revealSeenKey, "1");
      }
      return;
    }
    if (status !== "active") {
      setShowLiveReveal(false);
    }
  }, [deployment, hasLoadedRevealSeen, hasSeenReveal, revealSeenKey, status]);

  useEffect(() => {
    if (requestedTab !== "monitor" || allowMonitorTab || requestedFromAgents) return;
    const params = new URLSearchParams(location.search);
    params.delete("tab");
    const next = params.toString();
    navigate(`${location.pathname}${next ? `?${next}` : ""}`, { replace: true });
  }, [allowMonitorTab, location.pathname, location.search, navigate, requestedFromAgents, requestedTab]);

  if (!deployment) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/agents">My Agents</Link>
        </Button>
      </div>
    );
  }

  const backgroundDeployment = showLiveReveal
    ? { ...deployment, status: "pending", ready: 0 }
    : deployment;
  const backgroundInitialTab: "monitor" | "deployments" = showLiveReveal ? "deployments" : initialTab;
  const backgroundMonitorLocked = showLiveReveal ? true : monitorLocked;

  return (
    <>
      <ActiveDetailView
        key={`${deployment.id}-${backgroundInitialTab}-${showLiveReveal ? "reveal" : "normal"}`}
        deployment={backgroundDeployment}
        account={account}
        isPersonal={isPersonal}
        initialTab={backgroundInitialTab}
        monitorLocked={backgroundMonitorLocked}
      />
      {showLiveReveal && (
        <LiveRevealOverlay
          deployment={deployment}
          account={account}
          onViewMonitoring={() => {
            setAllowMonitorTab(true);
            const params = new URLSearchParams(location.search);
            params.set("tab", "monitor");
            navigate(`${location.pathname}?${params.toString()}`, { replace: true });
            setStayOnDeployments(false);
            setShowLiveReveal(false);
          }}
        />
      )}
    </>
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  if (!loaderData) {
    return <DeployedAgentDetailSkeleton />;
  }

  return (
    <ProtectedRoute>
      <DeployedAgentDetailContent loaderData={loaderData} />
    </ProtectedRoute>
  );
}
