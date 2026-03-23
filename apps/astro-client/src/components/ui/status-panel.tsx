import { useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { AlertCircle, CheckCircle2, Info, TriangleAlert, X } from "lucide-react";

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
            <Icon size={16} style={toneTextStyle} />
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
      <p className="text-sm whitespace-pre-wrap" style={toneTextStyle}>{children}</p>
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
