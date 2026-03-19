import { useEffect, useRef, useState } from 'react';
import { Link } from "react-router";
import { Rocket } from "lucide-react";
import { AgentIdentity } from "./AgentIdentity";
import { PrivacyBadge } from "@/components/PrivacyBadge";
import './AgentCard.css';

// ── Shared types ─────────────────────────────────────────────────
export type AgentTier = 'newHire' | 'proven' | 'expert' | 'elite';

export const TIER_CONFIG: Record<AgentTier, { label: string; rank: string; accent: string }> = {
  newHire: { label: 'NEW HIRE', rank: 'I',   accent: '#7e8e8d' },
  proven:  { label: 'PROVEN',   rank: 'II',  accent: '#15827d' },
  expert:  { label: 'EXPERT',   rank: 'III', accent: '#c4a96e' },
  elite:   { label: 'ELITE',    rank: 'IV',  accent: '#d4a017' },
};


const BARCODE_WIDTHS = [
  2,1,1,2,1,1,3,1,1,1,2,1,1,1,3,1,2,1,1,2,1,1,1,2,1,3,1,1,2,1,
  1,1,2,1,1,3,1,1,1,2,1,2,1,1,1,3,1,1,2,1,1,1,2,1,3,1,1,2,1,1,
  2,1,1,1,2,1,3,1,1,1,1,2,1,2,1,1,3,1,1,2,1,1,1,2,1,1,3,1,2,1,
];

function BarcodeStrip() {
  const rects: React.ReactElement[] = [];
  let x = 0;
  BARCODE_WIDTHS.forEach((w, i) => {
    if (i % 2 === 0) rects.push(<rect key={i} x={x} y={0} width={w} height={34} fill="currentColor" />);
    x += w;
  });
  return (
    <svg width="100%" height="34" viewBox={`0 0 ${x} 34`} preserveAspectRatio="none" style={{ color: '#57c4c1' }}>
      {rects}
    </svg>
  );
}

// ── Public props ─────────────────────────────────────────────────
export interface AgentCardProps {
  // user-badge variant
  variant?: 'default' | 'oftenUsedTogether' | 'user';
  tier?: AgentTier;
  account: string;
  name: string;
  displayName?: string;
  deploymentId?: string;
  stats?: { label: string; value: string }[];
  capabilities?: string[];
  // default / oftenUsedTogether only
  slug?: string;
  description?: string;
  visibility?: string;
  installs?: number;
}

// ── User-badge (hirer) card ───────────────────────────────────────
function UserBadgeCard({ account, name, displayName, tier = 'proven', deploymentId, stats, capabilities }: AgentCardProps) {
  const cardRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const [entered, setEntered] = useState(false);
  const rawName = (displayName || name).toUpperCase();
  const tierConfig = TIER_CONFIG[tier];

  useEffect(() => {
    const t1 = setTimeout(() => setVisible(true), 80);
    const t2 = setTimeout(() => setEntered(true), 80 + 780);
    return () => { clearTimeout(t1); clearTimeout(t2); };
  }, []);

  // Mouse tilt — direct DOM
  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!entered || !cardRef.current) return;
    const rect = cardRef.current.getBoundingClientRect();
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const rx = ((e.clientY - cy) / (rect.height / 2)) * -6;
    const ry = ((e.clientX - cx) / (rect.width / 2)) * 6;
    cardRef.current.style.transform = `perspective(900px) rotateX(${rx}deg) rotateY(${ry}deg) translateZ(10px)`;
    cardRef.current.style.transition = 'transform 0.08s ease';
  };

  const handleMouseLeave = () => {
    if (!cardRef.current) return;
    cardRef.current.style.transform = 'perspective(900px) rotateX(0deg) rotateY(0deg) translateZ(0px)';
    cardRef.current.style.transition = 'transform 0.65s cubic-bezier(0.16, 1, 0.3, 1)';
  };

  const statsRows = stats ?? [
    { label: 'TASKS',   value: '—' },
    { label: 'SUCCESS', value: '—' },
    { label: 'UPTIME',  value: '—' },
  ];

  const caps = capabilities ?? [];
  const idLabel = deploymentId ? `ID: ${deploymentId.toUpperCase()}` : `ID: ${name.slice(0, 10).toUpperCase()}`;

  return (
    <div
      ref={cardRef}
      className={`ac ac--user ${visible ? 'ac--in' : ''}`}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
      style={{ '--tier-accent': tierConfig.accent } as React.CSSProperties}
    >
      {/* Lanyard notch */}
      <div className="badge__notch" />

      {/* Tier clearance band */}
      <div className="badge__tier-band" style={{ background: tierConfig.accent }}>
        <span className="badge__tier-rank">{tierConfig.rank}</span>
        <span className="badge__tier-label">{tierConfig.label}</span>
        <span className="badge__tier-rank">{tierConfig.rank}</span>
      </div>

      {/* Body: main + side strip */}
      <div className="badge__body">
        <div className="badge__main">
          {/* Avatar */}
          <div className="badge__photo-section">
            <div className="ac__blob-glow" />
            <div className="ac__avatar-float">
              <div className="badge__photo-frame">
                <AgentIdentity account={account} name={name} size={140} />
                <div className="ac__scan" />
              </div>
            </div>
          </div>

          {/* Agent name */}
          <div className="badge__name-section">
            <div className="badge__name" style={{ opacity: visible ? 1 : 0, transition: 'opacity 0.3s ease 0.5s' }}>{rawName}</div>
          </div>

          {/* Stats rows */}
          <div className="badge__info">
            {statsRows.map((row) => (
              <div key={row.label} className="badge__info-row">
                <span className="badge__info-label">{row.label}</span>
                <span className="badge__info-value">{row.value}</span>
              </div>
            ))}
          </div>

          {/* Capabilities */}
          {caps.length > 0 && (
            <div className="ac__skills-section">
              <div className="ac__skills">
                {caps.map((cap) => (
                  <span key={cap} className="ac__skill">{cap.toUpperCase()}</span>
                ))}
              </div>
            </div>
          )}

          {/* Footer */}
          <div className="badge__footer">
            <div className="ac__attribution">
              <div className="ac__header-avatar" style={{ background: '#0D9488' }}>
                {account.slice(0, 2).toUpperCase()}
              </div>
              <div className="ac__attribution-info">
                <span className="ac__attribution-role">HIRED BY</span>
                <span className="ac__attribution-handle">@{account.toUpperCase()}</span>
              </div>
            </div>
            <div className="badge__barcode">
              <BarcodeStrip />
            </div>
            <div className="badge__id">{idLabel}</div>
          </div>
        </div>

        {/* Side strip */}
        <div className="badge__side-strip">
          <span className="badge__side-text">ORGANIZATION // {account.toUpperCase()}</span>
        </div>
      </div>
    </div>
  );
}

// ── Main export ───────────────────────────────────────────────────
export function AgentCard(props: AgentCardProps) {
  const { variant = 'default', slug, account, name, description, visibility, installs } = props;

  if (variant === 'user') {
    return <UserBadgeCard {...props} />;
  }

  const formattedInstalls = installs != null
    ? new Intl.NumberFormat('en-US').format(installs)
    : null;

  if (variant === 'oftenUsedTogether') {
    return (
      <Link
        to={`/${slug}`}
        className="group flex items-center gap-3 overflow-hidden rounded-md border border-border-strong bg-stone-200 px-3 py-2 transition-all duration-150 hover:bg-stone-300 hover:border-teal-500 hover:shadow-md dark:bg-muted/30 dark:hover:border-teal-400"
      >
        <AgentIdentity
          account={account}
          name={name}
          size={36}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="truncate text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            {name}
          </h3>
          <p className="font-mono text-mono-sm text-faint-foreground">
            {account}
          </p>
        </div>
      </Link>
    );
  }

  return (
    <Link
      to={`/${slug}`}
      className="group flex flex-col overflow-hidden rounded-md border border-stone-400 bg-stone-50 transition-all duration-150 hover:bg-stone-25 hover:border-teal-500 hover:shadow-md dark:bg-teal-900/30 dark:hover:border-teal-400"
    >
      <div className="flex flex-1 items-start gap-3 p-4 pb-3">
        <AgentIdentity
          account={account}
          name={name}
          size={36}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="flex flex-wrap items-center gap-1.5 text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            <span className="truncate">{name}</span>
            {visibility === 'private' && (
              <PrivacyBadge onClick={(e) => e.preventDefault()} />
            )}
          </h3>
          <p className="line-clamp-3 text-body-sm text-muted-foreground">
            {description}
          </p>
        </div>
      </div>
      <div className="flex items-center justify-between border-t border-border px-4 py-2.5">
        <span className="inline-flex items-center gap-1.5 text-mono-sm font-mono text-faint-foreground">
          <Rocket className="h-3 w-3" />
          {formattedInstalls ?? '1.2K'} deploys
        </span>
        <span className="text-mono-sm font-mono text-faint-foreground">
          {account}
        </span>
      </div>
    </Link>
  );
}
