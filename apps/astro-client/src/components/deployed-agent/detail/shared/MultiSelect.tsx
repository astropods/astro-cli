import { useEffect, useRef, useState } from "react";
import { ChevronDown } from "lucide-react";

const C = {
  bg: "var(--muted)",
  bgAlt: "var(--surface)",
  bgDeep: "var(--muted)",
  border: "var(--border)",
  teal: "var(--primary)",
  tealMid: "var(--color-teal-600)",
  muted: "var(--muted-foreground)",
  text: "var(--foreground)",
  faint: "var(--faint-foreground)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  body: "var(--text-body)",
  bodySm: "var(--text-body-sm)",
} as const;

const I = {
  xs: 10,
  sm: 12,
} as const;

interface MultiSelectOption {
  value: string;
  label: string;
  color?: string;
}

interface MultiSelectProps {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (v: string[]) => void;
  placeholder: string;
}

export function MultiSelect({ options, selected, onChange, placeholder }: MultiSelectProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [open]);

  const toggle = (v: string) => onChange(selected.includes(v) ? selected.filter((s) => s !== v) : [...selected, v]);
  const allSelected = selected.length === 0 || selected.length === options.length;
  const labelText = allSelected
    ? placeholder
    : selected.length === 1
      ? options.find((o) => o.value === selected[0])?.label ?? selected[0]
      : `${selected.length} selected`;

  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 6,
          padding: "5px 12px",
          minWidth: 134,
          borderRadius: 7,
          border: `1px solid ${open ? C.tealMid : C.border}`,
          background: open ? C.bgDeep : C.bg,
          cursor: "pointer",
          fontFamily: S.body,
          fontSize: T.body,
          color: allSelected ? C.muted : C.teal,
          transition: "all 0.12s",
          whiteSpace: "nowrap" as const,
        }}
      >
        <span>{labelText}</span>
        <ChevronDown
          size={I.sm}
          color={C.faint}
          style={{ transform: open ? "rotate(180deg)" : "none", transition: "transform 0.15s" }}
        />
      </button>
      {open && (
        <div
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            zIndex: 300,
            minWidth: 160,
            background: C.bgAlt,
            border: `1px solid ${C.border}`,
            borderRadius: 8,
            overflow: "hidden",
            boxShadow: "0 6px 20px rgba(0,0,0,0.1)",
          }}
        >
          <button
            onClick={() => onChange([])}
            style={{
              width: "100%",
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "8px 12px",
              background: "none",
              border: "none",
              borderBottom: `1px solid ${C.border}`,
              cursor: "pointer",
              fontFamily: S.body,
              fontSize: T.body,
              color: C.faint,
              textAlign: "left" as const,
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.background = C.bgDeep;
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = "none";
            }}
          >
            <div
              style={{
                width: 14,
                height: 14,
                borderRadius: 3,
                flexShrink: 0,
                border: `1.5px solid ${allSelected ? C.tealMid : C.border}`,
                background: allSelected ? C.tealMid : "transparent",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                transition: "all 0.12s",
              }}
            >
              {allSelected && (
                <svg width="8" height="8" viewBox="0 0 10 10">
                  <path
                    d="M2 5l2 2 4-4"
                    stroke="#fff"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    fill="none"
                  />
                </svg>
              )}
            </div>
            All
          </button>
          {options.map((opt) => {
            const checked = selected.includes(opt.value);
            return (
              <button
                key={opt.value}
                onClick={() => toggle(opt.value)}
                style={{
                  width: "100%",
                  display: "flex",
                  alignItems: "center",
                  gap: 8,
                  padding: "8px 12px",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  fontFamily: S.body,
                  fontSize: T.body,
                  color: C.text,
                  textAlign: "left" as const,
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = C.bgDeep;
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = "none";
                }}
              >
                <div
                  style={{
                    width: 14,
                    height: 14,
                    borderRadius: 3,
                    flexShrink: 0,
                    border: `1.5px solid ${checked ? C.tealMid : C.border}`,
                    background: checked ? C.tealMid : "transparent",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    transition: "all 0.12s",
                  }}
                >
                  {checked && (
                    <svg width="8" height="8" viewBox="0 0 10 10">
                      <path
                        d="M2 5l2 2 4-4"
                        stroke="#fff"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        fill="none"
                      />
                    </svg>
                  )}
                </div>
                {opt.color && (
                  <span
                    style={{
                      width: 7,
                      height: 7,
                      borderRadius: "50%",
                      background: opt.color,
                      display: "inline-block",
                      flexShrink: 0,
                    }}
                  />
                )}
                <span>{opt.label}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
