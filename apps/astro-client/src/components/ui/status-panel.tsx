import { useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { AlertCircle, CheckCircle2, Info, TriangleAlert, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export interface ErrorPanelProps {
  title?: string;
  children: ReactNode;
  dismissible?: boolean;
  onDismiss?: () => void;
  variant?: "default" | "inline";
}

interface DismissiblePanelProps {
  dismissible?: boolean;
  onDismiss?: () => void;
  variant?: "default" | "inline";
}

interface PanelToneConfig {
  textColor: string;
  backgroundColor: string;
  borderColor: string;
}

const PANEL_TONES: Record<"neutral" | "error" | "success" | "warning", PanelToneConfig> = {
  neutral: {
    textColor: "var(--color-blue-700)",
    backgroundColor: "color-mix(in oklch, var(--color-blue-700) 12%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-blue-700) 28%, transparent)",
  },
  error: {
    textColor: "var(--color-red-700)",
    backgroundColor: "color-mix(in oklch, var(--color-red-700) 12%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-red-700) 28%, transparent)",
  },
  success: {
    textColor: "var(--color-green-700)",
    backgroundColor: "color-mix(in oklch, var(--color-green-700) 12%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-green-700) 28%, transparent)",
  },
  warning: {
    textColor: "var(--color-yellow-700)",
    backgroundColor: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-yellow-700) 28%, transparent)",
  },
};

interface BasePanelProps {
  tone: "neutral" | "error" | "success" | "warning";
  title?: string;
  children: ReactNode;
  dismissible?: boolean;
  onDismiss?: () => void;
  variant?: "default" | "inline";
}

function BasePanel({ tone, title, children, dismissible = false, onDismiss, variant = "default" }: BasePanelProps) {
  const [dismissed, setDismissed] = useState(false);
  const toneConfig = PANEL_TONES[tone];
  const panelStyle: CSSProperties = {
    background: toneConfig.backgroundColor,
    border: `1px solid ${toneConfig.borderColor}`,
  };
  const toneTextStyle: CSSProperties = { color: toneConfig.textColor };

  if (dismissed) return null;

  const Icon = tone === "error"
    ? AlertCircle
    : tone === "success"
      ? CheckCircle2
      : tone === "warning"
        ? TriangleAlert
        : Info;

  if (variant === "inline") {
    return (
      <div className="rounded-[6px] px-4 py-2" style={panelStyle}>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <Icon size={16} className="shrink-0" style={toneTextStyle} />
            {title ? <span className="text-sm font-medium" style={toneTextStyle}>{title}</span> : null}
            {title ? <span className="text-sm" style={toneTextStyle}>-</span> : null}
            <span className="text-sm" style={toneTextStyle}>{children}</span>
          </div>
          {dismissible ? (
            <button
              type="button"
              aria-label="Dismiss panel"
              onClick={() => {
                setDismissed(true);
                onDismiss?.();
              }}
              className="shrink-0 rounded-sm p-0.5 hover:opacity-80"
              style={toneTextStyle}
            >
              <X size={14} />
            </button>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-[6px] p-4" style={panelStyle}>
      <div className="flex items-start justify-between gap-3">
        {title ? (
          <div className="flex items-center gap-1.5 mb-2">
            <Icon size={16} className="shrink-0" style={toneTextStyle} />
            <span className="text-sm font-medium" style={toneTextStyle}>{title}</span>
          </div>
        ) : (
          <div className="mb-2" />
        )}
        {dismissible ? (
          <button
            type="button"
            aria-label="Dismiss panel"
            onClick={() => {
              setDismissed(true);
              onDismiss?.();
            }}
            className="shrink-0 rounded-sm p-0.5 hover:opacity-80"
            style={toneTextStyle}
          >
            <X size={14} />
          </button>
        ) : null}
      </div>
      {children != null && <p className="text-sm whitespace-pre-wrap" style={toneTextStyle}>{children}</p>}
    </div>
  );
}

export function ErrorPanel({ title, children, dismissible = false, onDismiss, variant = "default" }: ErrorPanelProps) {
  return (
    <BasePanel tone="error" title={title} dismissible={dismissible} onDismiss={onDismiss} variant={variant}>
      {children}
    </BasePanel>
  );
}

export interface NeutralPanelProps extends DismissiblePanelProps {
  title?: string;
  children: ReactNode;
}

export function NeutralPanel({ title, children, dismissible = false, onDismiss, variant = "default" }: NeutralPanelProps) {
  return (
    <BasePanel tone="neutral" title={title} dismissible={dismissible} onDismiss={onDismiss} variant={variant}>
      {children}
    </BasePanel>
  );
}

export type InfoPanelProps = NeutralPanelProps;
export const InfoPanel = NeutralPanel;

export interface SuccessPanelProps extends DismissiblePanelProps {
  title?: string;
  children: ReactNode;
}

export function SuccessPanel({ title, children, dismissible = false, onDismiss, variant = "default" }: SuccessPanelProps) {
  return (
    <BasePanel tone="success" title={title} dismissible={dismissible} onDismiss={onDismiss} variant={variant}>
      {children}
    </BasePanel>
  );
}

export interface WarningPanelProps extends DismissiblePanelProps {
  title?: string;
  children: ReactNode;
}

export function WarningPanel({ title, children, dismissible = false, onDismiss, variant = "default" }: WarningPanelProps) {
  return (
    <BasePanel tone="warning" title={title} dismissible={dismissible} onDismiss={onDismiss} variant={variant}>
      {children}
    </BasePanel>
  );
}

export interface ActionPanelProps {
  title: ReactNode;
  /** Optional body content rendered below the title, aligned with the title text. */
  children?: ReactNode;
  primaryLabel: string;
  onPrimary: () => void;
  dismissible?: boolean;
  onDismiss?: () => void;
  confirmTitle?: string;
  confirmBody?: string;
  confirmLabel?: string;
  tone?: "neutral" | "error" | "warning";
}

/**
 * ActionPanel — info panel with a primary CTA.
 * If confirmTitle/confirmBody are provided, the primary action shows a
 * Dialog confirmation before firing.
 */
export function ActionPanel({
  title,
  children,
  primaryLabel,
  onPrimary,
  dismissible = false,
  onDismiss,
  confirmTitle,
  confirmBody,
  confirmLabel,
  tone = "neutral",
}: ActionPanelProps) {
  const [confirming, setConfirming] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const toneConfig = PANEL_TONES[tone] ?? PANEL_TONES.neutral;

  const Icon = tone === "warning" ? TriangleAlert
    : tone === "error" ? AlertCircle
    : Info;

  const buttonStyle: CSSProperties = { backgroundColor: toneConfig.textColor, color: "white", border: "none" };

  if (dismissed) return null;

  return (
    <>
      <div className="rounded-[6px] px-4 py-3" style={{ background: toneConfig.backgroundColor, border: `1px solid ${toneConfig.borderColor}` }}>
        <div className="flex items-center gap-3">
          <Icon size={16} className="shrink-0" style={{ color: toneConfig.textColor }} />
          <div className="flex-1 min-w-0">
            <span className="text-sm font-medium" style={{ color: toneConfig.textColor }}>{title}</span>
            {children && (
              <div className="mt-2 text-xs" style={{ color: toneConfig.textColor }}>
                {children}
              </div>
            )}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Button
              size="sm"
              variant="default"
              style={buttonStyle}
              className="hover:opacity-90 active:opacity-80"
              onClick={confirmTitle ? () => setConfirming(true) : onPrimary}
            >
              {primaryLabel}
            </Button>
            {dismissible && (
              <button
                type="button"
                aria-label="Dismiss"
                onClick={() => { setDismissed(true); onDismiss?.(); }}
                className="shrink-0 rounded-sm p-0.5 hover:opacity-80"
                style={{ color: toneConfig.textColor }}
              >
                <X size={14} />
              </button>
            )}
          </div>
        </div>
      </div>
      {confirmTitle && (
        <Dialog open={confirming} onOpenChange={setConfirming}>
          <DialogContent showCloseButton={false}>
            <DialogHeader>
              <DialogTitle>{confirmTitle}</DialogTitle>
              {confirmBody && <DialogDescription>{confirmBody}</DialogDescription>}
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirming(false)}>Cancel</Button>
              <Button
                variant="default"
                style={buttonStyle}
                className="hover:opacity-90 active:opacity-80"
                onClick={() => { setConfirming(false); onPrimary(); }}
              >
                {confirmLabel ?? primaryLabel}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
