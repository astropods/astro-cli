import { useEffect, useMemo, useState } from "react";
import { fetchIcons, iconUrl, processAssets } from "../api";
import type { IconEntry, IconsResponse } from "../types";

type Variant = "light" | "dark" | "both";

interface BuildState {
  status: "idle" | "running" | "ok" | "error";
  message?: string;
  durationMs?: number;
}

export function LibraryView({
  refreshKey,
  onRebuilt,
}: {
  refreshKey: number;
  onRebuilt: () => void;
}) {
  const [data, setData] = useState<IconsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [variant, setVariant] = useState<Variant>("both");
  const [filter, setFilter] = useState("");
  const [build, setBuild] = useState<BuildState>({ status: "idle" });

  useEffect(() => {
    setData(null);
    setError(null);
    fetchIcons().then(setData).catch((e) => setError(String(e)));
  }, [refreshKey]);

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    return q
      ? data.icons.filter((i) => i.id.toLowerCase().includes(q))
      : data.icons;
  }, [data, filter]);

  async function rebuild() {
    setBuild({ status: "running" });
    try {
      const r = await processAssets();
      setBuild({
        status: "ok",
        durationMs: r.durationMs,
        message: r.stdout?.split("\n").pop() || "done",
      });
      onRebuilt();
    } catch (e) {
      setBuild({ status: "error", message: (e as Error).message });
    }
  }

  return (
    <div className="p-6">
      <div className="flex items-center gap-3 mb-6">
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter…"
          className="flex-1 max-w-md bg-white/5 border border-white/10 rounded-md px-3 py-2 text-sm placeholder:text-white/30 focus:outline-none focus:border-white/30"
        />
        <VariantToggle value={variant} onChange={setVariant} />
        <RebuildButton state={build} onClick={rebuild} />
        <span className="text-xs text-white/40 ml-auto">
          {filtered.length} {filtered.length === 1 ? "icon" : "icons"}
        </span>
      </div>

      {error && (
        <div className="text-red-300/80 text-sm font-mono mb-4">{error}</div>
      )}

      {build.status === "ok" && (
        <div className="text-emerald-300/80 text-xs font-mono mb-4">
          ✓ {build.message} ({build.durationMs}ms)
        </div>
      )}
      {build.status === "error" && (
        <div className="text-red-300/80 text-xs font-mono mb-4 whitespace-pre-wrap">
          {build.message}
        </div>
      )}

      <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(180px,1fr))]">
        {filtered.map((icon) => (
          <IconTile key={icon.id} icon={icon} variant={variant} />
        ))}
      </div>
    </div>
  );
}

function RebuildButton({
  state,
  onClick,
}: {
  state: BuildState;
  onClick: () => void;
}) {
  const disabled = state.status === "running";
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title="Run scripts/process.ts — regenerates assets/integrations/light + dark from sources/"
      className={
        "px-3 py-1.5 rounded-md text-xs font-medium border transition-colors " +
        (disabled
          ? "border-white/10 text-white/40 cursor-not-allowed"
          : "border-white/15 text-white/80 hover:bg-white/5 hover:text-white")
      }
    >
      {state.status === "running" ? (
        <>
          <Spinner /> Rebuilding…
        </>
      ) : (
        "Rebuild assets"
      )}
    </button>
  );
}

function Spinner() {
  return (
    <span className="inline-block w-3 h-3 rounded-full border-2 border-white/30 border-t-white animate-spin align-[-2px] mr-1" />
  );
}

function VariantToggle({
  value,
  onChange,
}: {
  value: Variant;
  onChange: (v: Variant) => void;
}) {
  const opts: { v: Variant; label: string }[] = [
    { v: "light", label: "Light" },
    { v: "dark", label: "Dark" },
    { v: "both", label: "Both" },
  ];
  return (
    <div className="inline-flex rounded-md bg-white/5 border border-white/10 p-1 text-xs">
      {opts.map((o) => (
        <button
          key={o.v}
          onClick={() => onChange(o.v)}
          className={
            "px-2.5 py-1 rounded transition-colors " +
            (value === o.v
              ? "bg-white/15 text-white"
              : "text-white/60 hover:text-white")
          }
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

function IconTile({ icon, variant }: { icon: IconEntry; variant: Variant }) {
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.02] overflow-hidden">
      <div className="grid grid-cols-1 sm:grid-cols-2">
        {(variant === "light" || variant === "both") && (
          <SwatchPanel id={icon.id} variant="light" />
        )}
        {(variant === "dark" || variant === "both") && (
          <SwatchPanel id={icon.id} variant="dark" />
        )}
      </div>
      <div className="px-3 py-2 border-t border-white/10">
        <span className="font-mono text-xs truncate">{icon.id}</span>
      </div>
    </div>
  );
}

function SwatchPanel({
  id,
  variant,
}: {
  id: string;
  variant: "light" | "dark";
}) {
  const bg = variant === "light" ? "bg-white" : "bg-[#0b0c0f]";
  return (
    <div className={`flex items-center justify-center aspect-square ${bg}`}>
      <img
        src={iconUrl(id, variant)}
        alt={`${id} ${variant}`}
        className="w-12 h-12 object-contain"
        loading="lazy"
      />
    </div>
  );
}
