import { useEffect, useMemo, useState } from "react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { Share2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { HoloCard } from "./HoloCard";
import { ShareBadgeDropdown } from "./ShareBadgeDropdown";
import type { CardData } from "astro-trading-card";
import { generateCard } from "astro-trading-card";
import type { AvatarColors, ResolvedIntegration } from "@/lib/api";
import { useCardColors, useResolvedIntegrations } from "@/hooks/use-card-colors";

interface TradingCardModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data: CardData;
  avatarColors?: AvatarColors;
  integrations?: ResolvedIntegration[];
}

export function TradingCardModal({
  open,
  onOpenChange,
  data,
  avatarColors,
  integrations: rawIntegrations,
}: TradingCardModalProps) {
  const [entered, setEntered] = useState(false);
  const [showActions, setShowActions] = useState(false);

  const colors = useCardColors(avatarColors);
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
    () => generateCard(cardData),
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

  // Callers set qrUrl to the same `${origin}/${account}/${name}` shape we'd
  // build for the share intent, so reuse it (keeps share URL in lockstep with
  // the QR rendered on the card). Fallback only covers callers that pass a
  // CardData without qrUrl.
  const blueprintUrl = useMemo(() => {
    if (data.qrUrl) return data.qrUrl;
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return `${origin}/${data.account}/${data.name}`;
  }, [data.qrUrl, data.account, data.name]);

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
          onClick={() => onOpenChange(false)}
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
            onClick={(e) => e.stopPropagation()}
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
              "mt-5 flex w-[min(82vw,330px)] flex-col items-stretch transition-all duration-300 ease-out",
              showActions
                ? "opacity-100 translate-y-0"
                : "opacity-0 translate-y-4",
            )}
            onClick={(e) => e.stopPropagation()}
          >
            <ShareBadgeDropdown
              launchName={data.displayName ?? data.name}
              blueprintUrl={blueprintUrl}
              svg={svg}
              downloadName={data.name}
              downloadId={data.barcodeId ?? "card"}
              side="top"
            >
              <Button
                variant="outline"
                className="w-full gap-2 border-white/35 bg-transparent text-white shadow-none hover:border-white/55 hover:bg-white/10 hover:text-white dark:border-white/35 dark:bg-transparent dark:hover:border-white/55 dark:hover:bg-white/10"
              >
                <Share2 className="size-4" /> Share badge
              </Button>
            </ShareBadgeDropdown>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
