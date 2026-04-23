import { useMemo, useCallback, useRef, type PointerEvent as ReactPointerEvent } from "react";
import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { CapabilitiesList } from "./CapabilitiesList";
import { GitHubConnectionPanel } from "./GitHubConnectionPanel";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarRepository } from "./SidebarRepository";
import { SidebarSection } from "./SidebarSection";
import { SidebarStats } from "./SidebarStats";
import { BlueprintCard, type BlueprintCardProps } from "@/components/BlueprintCard";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import { getBlueprintRepository } from "@/lib/blueprint-utils";
import type { Blueprint, AccountPublic, BlueprintAuthor, ResolvedIntegration } from "@/lib/api";

const borderGlowStyle = {
  border: "1px solid transparent",
  background: "radial-gradient(circle 80px at var(--px, 50%) var(--py, 50%), rgba(255,255,255,0.7) 0%, rgba(255,255,255,0.1) 40%, transparent 70%) border-box",
  WebkitMask: "linear-gradient(#fff 0 0) padding-box, linear-gradient(#fff 0 0)",
  WebkitMaskComposite: "xor",
  maskComposite: "exclude",
} as const;

const noiseStyle = {
  backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='1.5' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`,
  backgroundRepeat: "repeat",
  backgroundSize: "200px 200px",
} as const;

export interface SidebarCardProps {
  agent: Blueprint;
  integrations: ResolvedIntegration[];
  capabilities?: string[];
  authors?: BlueprintAuthor[];
  rating?: number;
  installs?: number;
  recommendedAgents?: BlueprintCardProps[];
  initialAccountData?: AccountPublic;
  canEdit?: boolean;
  githubRepoName?: string;
  githubBranch?: string;
}

export function SidebarCard({
  agent,
  integrations,
  capabilities = [],
  authors = [],
  rating,
  installs,
  recommendedAgents = [],
  initialAccountData,
  canEdit,
  githubRepoName,
  githubBranch,
}: SidebarCardProps) {
  const latestVersion = agent.versions[0];
  const version = latestVersion?.version ?? latestVersion?.build_id?.slice(0, 8);
  const updatedAt = latestVersion?.published_at
    ? formatDate(latestVersion.published_at)
    : null;

  const { data: accountData } = useAccount(agent.account, {
    initialData: initialAccountData,
  });
  const owner = accountData?.owner;
  const ownerName = owner?.first_name && owner?.last_name
    ? `${owner.first_name} ${owner.last_name}`
    : agent.account;

  const repository = getBlueprintRepository(agent);
  const isDraft = agent.versions.length === 0;

  // Derive a saturated CTA color from the accent hex by extracting hue
  // and forcing high saturation + controlled lightness.
  const ctaColor = useMemo(() => {
    const hex = agent.avatar_colors?.accent;
    if (!hex) return null;
    const r = parseInt(hex.slice(1, 3), 16) / 255;
    const g = parseInt(hex.slice(3, 5), 16) / 255;
    const b = parseInt(hex.slice(5, 7), 16) / 255;
    const max = Math.max(r, g, b), min = Math.min(r, g, b);
    let h = 0;
    if (max !== min) {
      const d = max - min;
      switch (max) {
        case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
        case g: h = ((b - r) / d + 2) / 6; break;
        default: h = ((r - g) / d + 4) / 6; break;
      }
    }
    const hDeg = Math.round(h * 360);
    return {
      base: `hsl(${hDeg} 50% 45%)`,
      light: `hsl(${hDeg} 50% 55%)`,
      darkBase: `hsl(${hDeg} 50% 32%)`,
      darkLight: `hsl(${hDeg} 50% 42%)`,
    };
  }, [agent.avatar_colors?.accent]);

  const ctaStyleLight = ctaColor ? {
    backgroundColor: ctaColor.base,
  } : undefined;
  const ctaStyleDark = ctaColor ? {
    backgroundColor: ctaColor.darkBase,
  } : undefined;

  const lightBtnRef = useRef<HTMLAnchorElement>(null);
  const darkBtnRef = useRef<HTMLAnchorElement>(null);
  // Wrapper: updates cursor position + border glow opacity (--o)
  const handleWrapperPointerMove = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    for (const el of [lightBtnRef.current, darkBtnRef.current]) {
      if (!el) continue;
      const rect = el.getBoundingClientRect();
      const px = ((e.clientX - rect.left) / rect.width) * 100;
      const py = ((e.clientY - rect.top) / rect.height) * 100;
      el.style.setProperty("--px", `${px}%`);
      el.style.setProperty("--py", `${py}%`);
      el.style.setProperty("--fl", String(px / 100));
      el.style.setProperty("--o", "0.6");
    }
  }, []);
  const handleWrapperPointerLeave = useCallback(() => {
    lightBtnRef.current?.style.setProperty("--o", "0.25");
    darkBtnRef.current?.style.setProperty("--o", "0.25");
  }, []);
  // Direct button: controls holo shine/glare opacity (--ho)
  const handleBtnPointerEnter = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    e.currentTarget.style.setProperty("--ho", "0.6");
  }, []);
  const handleBtnPointerLeave = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    e.currentTarget.style.setProperty("--ho", "0");
  }, []);

  return (
    <div className="space-y-4">
      <div
        className="rounded-[4px] border border-border-strong bg-stone-200 p-4 dark:bg-muted/30"
        onPointerMove={ctaColor ? handleWrapperPointerMove : undefined}
        onPointerLeave={ctaColor ? handleWrapperPointerLeave : undefined}
      >
        {isDraft ? (
          <Button size="default" className="h-11 w-full" disabled>
            Deploy this agent
            <ArrowRight className="h-4 w-4" />
          </Button>
        ) : (
          <>
            {/* Light-mode CTA */}
            <Button
              asChild
              size="default"
              className="relative h-11 w-full overflow-hidden text-white dark:hidden"
              style={ctaStyleLight}
            >
              <Link
                ref={lightBtnRef}
                to={`/deploy/${agent.account}/${agent.name}`}
                onPointerEnter={ctaColor ? handleBtnPointerEnter : undefined}
                onPointerLeave={ctaColor ? handleBtnPointerLeave : undefined}
              >
                {ctaColor && (
                  <>
                    {/* Border glow — follows cursor, driven by wrapper --o */}
                    <span
                      className="pointer-events-none absolute inset-[1px] rounded-[inherit] transition-opacity duration-300"
                      style={{ ...borderGlowStyle, opacity: "var(--o, 0.25)" as unknown as number }}
                    />
                    <span className="pointer-events-none absolute inset-0 opacity-30 mix-blend-overlay" style={noiseStyle} />
                    {/* Shine — rainbow color-dodge layer, driven by direct --ho */}
                    <span
                      className="pointer-events-none absolute inset-0 transition-opacity duration-300"
                      style={{
                        opacity: "var(--ho, 0)" as unknown as number,
                        mixBlendMode: "color-dodge",
                        backgroundImage: [
                          "radial-gradient(circle at var(--px,30%) var(--py,40%), #fff 5%, #000 50%, #fff 80%)",
                          "linear-gradient(-45deg, #000 15%, #fff, #000 85%)",
                          "repeating-linear-gradient(135deg, hsl(280,80%,50%) 0%, hsl(200,80%,50%) 10%, hsl(140,80%,50%) 20%, hsl(60,80%,50%) 30%, hsl(330,80%,50%) 40%, hsl(280,80%,50%) 50%)",
                        ].join(","),
                        backgroundBlendMode: "soft-light, difference",
                        backgroundSize: "120% 120%, 200% 200%, 150% 150%",
                        backgroundPosition: "center center, calc(100% * var(--fl, 0.3)) 50%, center center",
                        filter: "brightness(0.5) contrast(1.5) saturate(0.8)",
                      }}
                    />
                    {/* Glare — specular overlay, driven by direct --ho */}
                    <span
                      className="pointer-events-none absolute inset-0 mix-blend-overlay transition-opacity duration-300"
                      style={{
                        opacity: "var(--ho, 0)" as unknown as number,
                        backgroundImage: "radial-gradient(farthest-corner circle at var(--px,30%) var(--py,40%), hsla(0,0%,100%,0.8) 10%, hsla(0,0%,100%,0.5) 20%, hsla(0,0%,0%,0.75) 90%)",
                        filter: "brightness(0.7) contrast(1.5)",
                      }}
                    />
                  </>
                )}
                Deploy this agent
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            {/* Dark-mode CTA */}
            <Button
              asChild
              size="default"
              className="relative hidden h-11 w-full overflow-hidden text-white dark:flex"
              style={ctaStyleDark}
            >
              <Link
                ref={darkBtnRef}
                to={`/deploy/${agent.account}/${agent.name}`}
                onPointerEnter={ctaColor ? handleBtnPointerEnter : undefined}
                onPointerLeave={ctaColor ? handleBtnPointerLeave : undefined}
              >
                {ctaColor && (
                  <>
                    {/* Border glow — follows cursor, driven by wrapper --o */}
                    <span
                      className="pointer-events-none absolute inset-[1px] rounded-[inherit] transition-opacity duration-300"
                      style={{ ...borderGlowStyle, opacity: "var(--o, 0.25)" as unknown as number }}
                    />
                    <span className="pointer-events-none absolute inset-0 opacity-30 mix-blend-overlay" style={noiseStyle} />
                    {/* Shine, driven by direct --ho */}
                    <span
                      className="pointer-events-none absolute inset-0 transition-opacity duration-300"
                      style={{
                        opacity: "var(--ho, 0)" as unknown as number,
                        mixBlendMode: "color-dodge",
                        backgroundImage: [
                          "radial-gradient(circle at var(--px,30%) var(--py,40%), #fff 5%, #000 50%, #fff 80%)",
                          "linear-gradient(-45deg, #000 15%, #fff, #000 85%)",
                          "repeating-linear-gradient(135deg, hsl(280,80%,50%) 0%, hsl(200,80%,50%) 10%, hsl(140,80%,50%) 20%, hsl(60,80%,50%) 30%, hsl(330,80%,50%) 40%, hsl(280,80%,50%) 50%)",
                        ].join(","),
                        backgroundBlendMode: "soft-light, difference",
                        backgroundSize: "120% 120%, 200% 200%, 150% 150%",
                        backgroundPosition: "center center, calc(100% * var(--fl, 0.3)) 50%, center center",
                        filter: "brightness(0.5) contrast(1.5) saturate(0.8)",
                      }}
                    />
                    {/* Glare, driven by direct --ho */}
                    <span
                      className="pointer-events-none absolute inset-0 mix-blend-overlay transition-opacity duration-300"
                      style={{
                        opacity: "var(--ho, 0)" as unknown as number,
                        backgroundImage: "radial-gradient(farthest-corner circle at var(--px,30%) var(--py,40%), hsla(0,0%,100%,0.8) 10%, hsla(0,0%,100%,0.5) 20%, hsla(0,0%,0%,0.75) 90%)",
                        filter: "brightness(0.7) contrast(1.5)",
                      }}
                    />
                  </>
                )}
                Deploy this agent
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          </>
        )}
      </div>

      {canEdit && (
        <GitHubConnectionPanel account={agent.account} name={agent.name} preConnectedRepo={githubRepoName} preConnectedBranch={githubBranch} />
      )}

      <SidebarStats
        rating={rating}
        installs={installs}
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
        visibility={agent.visibility}
        isDraft={isDraft}
      />

      <SidebarAuthor
        authors={authors}
        ownerName={ownerName}
        ownerHandle={agent.account}
      />

      {repository && <SidebarRepository repository={repository} />}

      {integrations.length > 0 && (
        <RequiredAppsList integrations={integrations} title="Integrations" />
      )}

      {capabilities.length > 0 && (
        <CapabilitiesList capabilities={capabilities} />
      )}

      {recommendedAgents.length > 0 && (
        <SidebarSection title="More blueprints">
          <div className="space-y-2.5">
            {recommendedAgents.map((recommendedAgent) => (
              <BlueprintCard
                key={recommendedAgent.slug}
                {...recommendedAgent}
                variant="oftenUsedTogether"
              />
            ))}
          </div>
        </SidebarSection>
      )}

    </div>
  );
}

export type BlueprintDetailSidebarProps = SidebarCardProps;

export function BlueprintDetailSidebar(props: BlueprintDetailSidebarProps) {
  return (
    <div className="hidden min-[900px]:block w-[340px] shrink-0 pl-0 pr-8 pt-10 pb-6">
      <SidebarCard {...props} />
    </div>
  );
}
