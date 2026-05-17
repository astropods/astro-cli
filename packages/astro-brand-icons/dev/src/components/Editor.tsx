import { useEffect, useState } from "react";
import { saveIcon } from "../api";

export function Editor({
  initialId,
  initialLightSvg,
  initialDarkSvg,
  onSaved,
}: {
  initialId: string;
  initialLightSvg: string;
  initialDarkSvg: string;
  onSaved: (id: string) => void;
}) {
  const [id, setId] = useState(initialId);
  const [lightSvg, setLightSvg] = useState(initialLightSvg);
  const [darkSvg, setDarkSvg] = useState(initialDarkSvg);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setId(initialId);
    setLightSvg(initialLightSvg);
    setDarkSvg(initialDarkSvg);
    setError(null);
  }, [initialId, initialLightSvg, initialDarkSvg]);

  async function onSave() {
    const trimmedId = id.trim();
    setSaving(true);
    setError(null);
    try {
      await saveIcon({ id: trimmedId, lightSvg, darkSvg });
      onSaved(trimmedId);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  const canSave =
    id.trim().length > 0 &&
    lightSvg.trim().startsWith("<svg") &&
    darkSvg.trim().startsWith("<svg") &&
    !saving;

  return (
    <div className="rounded-lg border border-white/10 p-5 bg-white/[0.02]">
      <div className="flex items-end gap-4 mb-5">
        <div className="flex-1 max-w-md">
          <Label>ID</Label>
          <input
            value={id}
            onChange={(e) => setId(e.target.value)}
            className="w-full bg-white/5 border border-white/10 rounded-md px-3 py-2 text-sm font-mono"
          />
          <p className="text-[11px] text-white/40 mt-1">
            Will write{" "}
            <span className="font-mono">{id || "—"}.svg</span> and{" "}
            <span className="font-mono">{id || "—"}.dark.svg</span> to{" "}
            <span className="font-mono">sources/</span>.
          </p>
        </div>
        <div className="flex-1 flex justify-end gap-2">
          {error && (
            <span className="text-xs text-red-300/80 font-mono self-center">
              {error}
            </span>
          )}
          <button
            type="button"
            onClick={onSave}
            disabled={!canSave}
            className="px-4 py-2 rounded-md bg-white text-black text-sm font-medium hover:bg-white/90 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {saving ? "Saving…" : "Save to package"}
          </button>
        </div>
      </div>

      <div className="grid md:grid-cols-2 gap-5">
        <VariantPane
          label="Light"
          bg="bg-white"
          svg={lightSvg}
          onChange={setLightSvg}
        />
        <VariantPane
          label="Dark"
          bg="bg-[#0b0c0f]"
          svg={darkSvg}
          onChange={setDarkSvg}
        />
      </div>
    </div>
  );
}

function VariantPane({
  label,
  bg,
  svg,
  onChange,
}: {
  label: string;
  bg: string;
  svg: string;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>{label}</Label>
      <div
        className={`rounded-md overflow-hidden border border-white/10 ${bg} aspect-[3/1] flex items-center justify-center p-6`}
      >
        <Preview svg={svg} />
      </div>
      <textarea
        value={svg}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
        className="mt-2 w-full h-48 bg-black/30 border border-white/10 rounded-md p-2 font-mono text-[11px] leading-snug resize-y"
      />
    </div>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-[11px] uppercase tracking-wider text-white/50 mb-1.5">
      {children}
    </div>
  );
}

function Preview({ svg }: { svg: string }) {
  return (
    <div
      className="w-full h-full [&>svg]:w-full [&>svg]:h-full [&>svg]:max-w-16 [&>svg]:max-h-16 flex items-center justify-center"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
