import { useEffect, useRef, useState } from "react";
import { MoreVertical, Copy, Check, Pencil, Trash2 } from "lucide-react";

const C = {
  bgAlt: "var(--surface)",
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

export function KebabMenu({ deploymentId }: { deploymentId: string }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

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

  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          background: "none",
          border: "none",
          cursor: "pointer",
          color: C.faint,
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
        <MoreVertical size={I.md} />
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
          {[
            {
              icon: copied ? Check : Copy,
              label: copied ? "Copied!" : "Copy ID number",
              color: C.text,
              onClick: copyId,
              sep: false,
            },
            { icon: Pencil, label: "Rename", color: C.text, onClick: () => setOpen(false), sep: false },
            { icon: Trash2, label: "Delete agent", color: C.coral, onClick: () => setOpen(false), sep: true },
          ].map(({ icon: Icon, label, color, onClick, sep }) => (
            <div key={label}>
              {sep && <div style={{ height: 1, background: C.border }} />}
              <button
                style={{
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
                  color,
                  textAlign: "left" as const,
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
                onClick={onClick}
              >
                <Icon size={I.md} />
                {label}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
