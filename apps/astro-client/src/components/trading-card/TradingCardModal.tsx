import { useEffect, useState, useMemo } from "react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { Download, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { HoloCard } from "./HoloCard";
import type { CardColors, CardData } from "astro-trading-card";
import { generateCard, extractPalette, pickCardColors } from "astro-trading-card";
import type { ResolvedIntegration } from "@/lib/api";
import { resolveCardIntegrations } from "./integrationIcons";

const DEFAULT_COLORS: CardColors = {
  background: "#0d0d12",
  foreground: "#f0f0f4",
  accent: "#6366f1",
  accentLight: "#a5b4fc",
  glow: "#b4bfff",
};

function useExtractedColors(avatar: CardData["avatar"], open: boolean) {
  const [colors, setColors] = useState<CardColors>(DEFAULT_COLORS);

  useEffect(() => {
    if (!open) return;

    let cancelled = false;

    let source: string | null = null;
    if (avatar?.url) {
      source = avatar.url;
    } else if (avatar?.svg) {
      const full = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">${avatar.svg}</svg>`;
      source = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(full);
    }

    if (!source) {
      setColors(DEFAULT_COLORS);
      return;
    }

    const img = new Image();
    img.crossOrigin = "anonymous";
    img.src = source;
    img.onload = () => {
      if (cancelled) return;
      const canvas = document.createElement("canvas");
      canvas.width = 64;
      canvas.height = 64;
      const ctx = canvas.getContext("2d")!;
      ctx.drawImage(img, 0, 0, 64, 64);
      const { data } = ctx.getImageData(0, 0, 64, 64);
      const palette = extractPalette(data, 8, 1);
      const result = pickCardColors(palette);
      if (!cancelled) setColors(result ?? DEFAULT_COLORS);
    };
    img.onerror = () => {
      if (!cancelled) setColors(DEFAULT_COLORS);
    };

    return () => { cancelled = true; };
  }, [avatar, open]);

  return colors;
}

function useResolvedIntegrations(
  integrations: ResolvedIntegration[] | undefined,
  open: boolean,
) {
  return useMemo(
    () => (open && integrations?.length ? resolveCardIntegrations(integrations) : []),
    [integrations, open],
  );
}

interface TradingCardModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data: CardData;
  integrations?: ResolvedIntegration[];
}

export function TradingCardModal({
  open,
  onOpenChange,
  data,
  integrations: rawIntegrations,
}: TradingCardModalProps) {
  const [entered, setEntered] = useState(false);
  const [showActions, setShowActions] = useState(false);

  const colors = useExtractedColors(data.avatar, open);
  const cardIntegrations = useResolvedIntegrations(rawIntegrations, open);

  const cardData = useMemo<CardData>(
    () => ({
      ...data,
      colors,
      ...(cardIntegrations.length > 0 ? { integrations: cardIntegrations } : {}),
    }),
    [data, colors, cardIntegrations],
  );

  const svg = useMemo(
    () => generateCard(cardData, { variant: "standard" }),
    [cardData],
  );

  useEffect(() => {
    if (open) {
      setEntered(false);
      setShowActions(false);
      const t1 = setTimeout(() => setEntered(true), 50);
      const t2 = setTimeout(() => setShowActions(true), 400);
      return () => { clearTimeout(t1); clearTimeout(t2); };
    }
    setEntered(false);
    setShowActions(false);
  }, [open]);

  const handleDownload = async (format: "svg" | "png") => {
    const mod = await import("astro-trading-card/browser");
    const opts = { name: data.name, id: data.barcodeId ?? "card" };
    if (format === "svg") {
      await mod.downloadSvg(svg, opts);
    } else {
      await mod.downloadPng(svg, opts);
    }
  };

  if (!open) return null;

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className={cn(
            "fixed inset-0 z-50 bg-black/70 backdrop-blur-md",
            "data-[state=closed]:animate-out data-[state=closed]:fade-out-0",
            "data-[state=open]:animate-in data-[state=open]:fade-in-0",
          )}
        />

        <DialogPrimitive.Content
          className="fixed inset-0 z-50 flex flex-col items-center justify-center outline-none"
          onPointerDownOutside={() => onOpenChange(false)}
        >
          <button
            onClick={() => onOpenChange(false)}
            className="absolute top-6 right-6 rounded-full p-2 text-white/60 hover:text-white hover:bg-white/10 transition-colors z-10"
          >
            <X className="size-5" />
          </button>

          <div
            className={cn(
              "transition-all duration-500 ease-out",
              entered
                ? "opacity-100 translate-y-0"
                : "opacity-0 -translate-y-12",
            )}
          >
            <HoloCard>
              <div
                style={{ borderRadius: 16, overflow: "hidden" }}
                dangerouslySetInnerHTML={{ __html: svg }}
              />
            </HoloCard>
          </div>

          <div
            className={cn(
              "mt-8 flex gap-3 transition-all duration-300 ease-out",
              showActions
                ? "opacity-100 translate-y-0"
                : "opacity-0 translate-y-4",
            )}
          >
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDownload("svg")}
              className="gap-2 border-white/20 text-white hover:bg-white/10 hover:text-white dark:border-white/20 dark:hover:bg-white/10"
            >
              <Download className="size-3.5" />
              SVG
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDownload("png")}
              className="gap-2 border-white/20 text-white hover:bg-white/10 hover:text-white dark:border-white/20 dark:hover:bg-white/10"
            >
              <Download className="size-3.5" />
              PNG
            </Button>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
