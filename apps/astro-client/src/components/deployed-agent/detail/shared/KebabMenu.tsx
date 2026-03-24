import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { EllipsisHorizontalIcon, DocumentDuplicateIcon, CheckIcon, TrashIcon, BookOpenIcon, ShareIcon } from "@heroicons/react/24/outline";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { useAgent } from "@/api/queries/agents";
import { getAgentIntegrations } from "@/lib/agent-utils";
import type { CardData, CardAvatar } from "astro-trading-card";
import { stripSvgWrapper } from "astro-trading-card";
import { generateIdentity } from "identity-gen";

const C = {
  bgAlt: "var(--popover)",
  bgDeep: "var(--muted)",
  panel: "var(--surface)",
  border: "var(--border)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  faint: "var(--faint-foreground)",
  coral: "var(--color-coral-600)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  heading4: "var(--text-heading-4)",
} as const;

const I = {
  md: 14,
} as const;

interface KebabMenuProps {
  deploymentId: string;
  deploymentName: string;
  displayName?: string;
  account: string;
  installedAt?: string;
  avatarUrl?: string;
  onDeleted?: () => void;
}

export function KebabMenu({ deploymentId, deploymentName, displayName, account, installedAt, avatarUrl, onDeleted }: KebabMenuProps) {
  const [open, setOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const { data: agent } = useAgent(account, deploymentName, { enabled: shareOpen });
  const integrations = agent ? getAgentIntegrations(agent) : [];

  const cardAvatar = useMemo<CardAvatar | undefined>(() => {
    if (avatarUrl) return { url: avatarUrl };
    const svg = generateIdentity({ seed: `${account}/${deploymentName}`, size: 128 });
    return { svg: stripSvgWrapper(svg) };
  }, [avatarUrl, account, deploymentName]);

  const cardData = useMemo<CardData>(() => ({
    name: deploymentName,
    displayName,
    account,
    avatar: cardAvatar,
    stats: [
      { label: "Deployed", value: installedAt ?? "" },
      { label: "From", value: `${account}/${deploymentName}` },
    ],
    barcodeId: deploymentId,
    qrUrl: `${window.location.origin}/${account}/${deploymentName}`,
  }), [deploymentName, displayName, account, cardAvatar, installedAt, deploymentId]);

  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [open]);

  const copyId = () => {
    navigator.clipboard.writeText(deploymentId);
    setCopied(true);
    setTimeout(() => {
      setCopied(false);
      setOpen(false);
    }, 1600);
  };

  const linkStyle = {
    width: "100%",
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "10px 14px",
    background: "none",
    fontFamily: S.body,
    fontSize: T.heading4,
    color: C.text,
    textDecoration: "none",
  } as const;

  const buttonStyle = {
    width: "100%",
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "10px 14px",
    background: "none",
    border: "none",
    cursor: "pointer",
    fontFamily: S.body,
    fontSize: T.heading4,
    color: C.text,
    textAlign: "left" as const,
  } as const;

  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          background: "none",
          border: "none",
          cursor: "pointer",
          color: C.text,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          width: 28,
          height: 28,
          borderRadius: 6,
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
        onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
      >
        <EllipsisHorizontalIcon style={{ width: I.md, height: I.md }} />
      </button>
      {open && (
        <div
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            zIndex: 100,
            minWidth: 180,
            background: C.bgAlt,
            border: `1px solid ${C.border}`,
            borderRadius: 10,
            overflow: "hidden",
            boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
          }}
        >
          <Link
            to={`/${account}/${deploymentName}`}
            onClick={() => setOpen(false)}
            style={linkStyle}
            onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
            onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
          >
            <BookOpenIcon style={{ width: I.md, height: I.md }} />
            View blueprint
          </Link>
          <button
            style={buttonStyle}
            onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
            onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
            onClick={() => { setOpen(false); setShareOpen(true); }}
          >
            <ShareIcon style={{ width: I.md, height: I.md }} />
            Share agent badge
          </button>
          {[
            {
              icon: copied ? CheckIcon : DocumentDuplicateIcon,
              label: copied ? "Copied!" : "Copy build number",
              color: C.text,
              onClick: copyId,
              sep: false,
            },
            {
              icon: TrashIcon,
              label: "Delete agent",
              color: C.coral,
              onClick: () => {
                setOpen(false);
                setDeleteOpen(true);
              },
              sep: true,
            },
          ].map(({ icon: Icon, label, color, onClick, sep }) => (
            <div key={label}>
              {sep && <div style={{ height: 1, background: C.border }} />}
              <button
                style={{ ...buttonStyle, color }}
                onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
                onClick={onClick}
              >
                <Icon style={{ width: I.md, height: I.md }} />
                {label}
              </button>
            </div>
          ))}
        </div>
      )}
      <DeleteDeploymentDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        deploymentId={deploymentId}
        deploymentName={deploymentName}
        displayName={displayName}
        account={account}
        onDeleted={onDeleted}
      />
      <TradingCardModal
        open={shareOpen}
        onOpenChange={setShareOpen}
        data={cardData}
        integrations={integrations}
      />
    </div>
  );
}
