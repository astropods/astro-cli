import { useEffect, useRef, useState } from "react";
import { MoreVertical, Copy, Check, Pencil, Trash2 } from "lucide-react";
import { C, S } from "../theme";

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
        <MoreVertical size={15} />
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
                  fontSize: 13,
                  color,
                  textAlign: "left" as const,
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={(e) => (e.currentTarget.style.background = "none")}
                onClick={onClick}
              >
                <Icon size={13} />
                {label}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
