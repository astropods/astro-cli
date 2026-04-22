import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { EllipsisHorizontalIcon, DocumentDuplicateIcon, CheckIcon, TrashIcon, BookOpenIcon, ShareIcon, ArrowPathIcon } from "@heroicons/react/24/outline";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import type { CardData } from "astro-trading-card";
import type { AvatarColors } from "@/lib/api";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

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
  avatarColors?: AvatarColors;
  onDeleted?: () => void;
  onRestart?: () => void;
  hideDestructive?: boolean;
}

export function KebabMenu({ deploymentId, deploymentName, displayName, account, installedAt, avatarColors, onDeleted, onRestart, hideDestructive = false }: KebabMenuProps) {
  const [open, setOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [restartConfirm, setRestartConfirm] = useState(false);
  const { copy: copyToClipboard, copied } = useCopyToClipboard(1600);
  const ref = useRef<HTMLDivElement>(null);

  const { data: agent } = useBlueprint(account, deploymentName, { enabled: shareOpen });
  const integrations = agent ? getBlueprintIntegrations(agent) : [];

  const deploymentAvatarBust = useDeploymentAvatarBust(deploymentId);
  const deploymentAvatarUrl = deploymentAvatarBust ?? getDeploymentAvatarUrl(deploymentId);

  const cardData = useMemo<CardData>(() => ({
    name: deploymentName,
    displayName,
    account,
    avatar: { url: deploymentAvatarUrl },
    stats: [
      { label: "Deployed", value: installedAt ?? "" },
      { label: "From", value: `${account}/${deploymentName}` },
    ],
    barcodeId: deploymentId,
    qrUrl: `${window.location.origin}/${account}/${deploymentName}`,
  }), [deploymentName, displayName, account, deploymentAvatarUrl, installedAt, deploymentId]);

  useEffect(() => {
    if (!open) { return; }
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [open]);

  const copyId = () => {
    void copyToClipboard(deploymentId);
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
    whiteSpace: "nowrap" as const,
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
          className="shadow-lg"
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
          <button
            style={{ ...buttonStyle, color: C.text }}
            onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
            onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
            onClick={copyId}
          >
            {copied ? <CheckIcon style={{ width: I.md, height: I.md }} /> : <DocumentDuplicateIcon style={{ width: I.md, height: I.md }} />}
            {copied ? "Copied!" : "Copy deploy ID"}
          </button>

          {!hideDestructive && onRestart && (
            <>
              <div style={{ height: 1, background: C.border }} />
              <button
                style={{ ...buttonStyle, color: C.coral }}
                onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
                onClick={() => { setOpen(false); setRestartConfirm(true); }}
              >
                <ArrowPathIcon style={{ width: I.md, height: I.md }} />
                Restart deployment
              </button>
            </>
          )}
          {!hideDestructive && (
            <>
              <div style={{ height: 1, background: C.border }} />
              <button
                style={{ ...buttonStyle, color: C.coral }}
                onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
                onClick={() => { setOpen(false); setDeleteOpen(true); }}
              >
                <TrashIcon style={{ width: I.md, height: I.md }} />
                Delete agent
              </button>
            </>
          )}
        </div>
      )}
      <Dialog open={restartConfirm} onOpenChange={(o) => { if (!o) setRestartConfirm(false); }}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Are you sure?</DialogTitle>
            <DialogDescription>All running containers will be restarted. There may be a brief interruption.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestartConfirm(false)}>Cancel</Button>
            <Button variant="destructive" onClick={() => { setRestartConfirm(false); onRestart?.(); }}>Restart</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
        avatarColors={avatarColors}
        integrations={integrations}
      />
    </div>
  );
}
